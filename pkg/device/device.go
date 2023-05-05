/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package device

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

var (
	lastCleanExecuted      time.Time
	lastStaleCleanExecuted time.Time
)

const (
	// deviceInfoFileName is used to store the device details in a JSON file
	deviceInfoFileName = "deviceInfo.json"
)

type LinuxDevice interface {
	GetDevice() bool
	DeleteDevice() (err error)
	CreateDevice() (err error)
	GetMapper() string
}

// Device struct
type Device struct {
	Mapper string   `json:"mapper,omitempty"`
	WWID   string   `json:"wwid,omitempty"`
	WWN    string   `json:"wwn,omitempty"`
	Slaves []string `json:"slaves,omitempty"`
}

// StagingDevice represents the device information that is stored in the staging area.
type StagingDevice struct {
	VolumeID         string      `json:"volume_id"`
	VolumeAccessMode string      `json:"volume_access_mode"` // block or mount
	Device           LinuxDevice `json:"device"`
	IsMounted        bool
}

func NewLinuxDevice(wwn string) LinuxDevice {
	return &Device{
		WWN: wwn,
	}
}

func (d *Device) GetMapper() string {
	return d.Mapper
}

// GetDevice: find the device with given wwn
func (d *Device) GetDevice() bool {
	err := d.getLinuxDmDevice(false)
	if err != nil || d.Mapper == "" {
		return false
	}
	return true
}

// getLinuxDmDevice: get all linux Devices
func (d *Device) getLinuxDmDevice(needActivePath bool) error {
	args := []string{"ls", "--target", "multipath"}
	outBytes, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
	out := string(outBytes)
	if err != nil {
		return fmt.Errorf("failed to retrieve multipath devices: %s", out)
	}

	r, err := regexp.Compile(majorMinorPattern)
	if err != nil {
		return fmt.Errorf("unable to compile regex with %s", majorMinorPattern)
	}
	listOut := r.FindAllString(out, -1)

	for _, line := range listOut {
		result := findStringSubmatchMap(line, r)

		tmpPathname := "dm-" + result["Minor"]
		uuid, err := getUUID(tmpPathname)
		if err != nil {
			return err
		}
		tmpWWID := strings.TrimPrefix(uuid, "mpath-")
		tmpWWN := tmpWWID[1:] // truncate scsi-id prefix

		if strings.EqualFold(d.WWN, tmpWWN) {
			d.WWID = tmpWWID
			mapName, err := getMpathName(tmpPathname)
			if err != nil {
				return err
			}
			d.Mapper = "/dev/mapper/" + mapName

			multipathdShowPaths, err := retryGetPathOfDevice(d, needActivePath)
			if err != nil {
				err = fmt.Errorf("unable to get scsi slaves for device %s: %v", d.WWN, err)
				return err
			}
			var slaves []string
			for _, path := range multipathdShowPaths {
				if path != nil && (!needActivePath || path.ChkState == "ready") {
					slaves = append(slaves, path.Device)
				}
			}
			d.Slaves = slaves
			break
		}
	}

	return nil
}

// DeleteDevice: delete the multipath device
func (d *Device) DeleteDevice() (err error) {
	err = tearDownMultipathDevice(d)
	if err != nil {
		return err
	}
	d.Mapper = ""
	d.Slaves = nil
	return nil
}

// CreateDevice: attach and create linux devices to host
func (d *Device) CreateDevice() (err error) {
	err = scsiHostRescan()
	if err != nil {
		return err
	}
	// wait for device to appear after rescan
	time.Sleep(time.Second * 1)

	err = d.createLinuxDevice()
	if err != nil {
		klog.Errorf("unable to create device for wwn %v", d.WWN)
		return err
	}

	if d.Mapper == "" {
		return fmt.Errorf("unable to find the device for wwn %s", d.WWN)
	}
	return nil
}

// createLinuxDevice: attaches and creates a new linux device
func (d *Device) createLinuxDevice() (err error) {
	// Start a Countdown ticker
	for i := 0; i <= 10; i++ {
		err := d.getLinuxDmDevice(true)
		if err != nil {
			return err
		}
		if len(d.Slaves) > 0 {
			// populated device with atleast 1 slave; job done
			return nil
		} else {
			// no slaves; cannot use this device; retry
			d.Mapper = ""
		}

		if i%2 == 0 {
			// try cleaning up faulty, orphan, stale paths/maps
			tmpTime := time.Now().Add(-10 * time.Second)
			if lastCleanExecuted.Before(tmpTime) {
				tryCleaningFaultyAndOrphan()
				lastCleanExecuted = time.Now()
			}
		} else {
			// handle stale paths
			// heavy operation hence try only between 25 secs
			tmpTime := time.Now().Add(-25 * time.Second)
			if lastStaleCleanExecuted.Before(tmpTime) {
				err = cleanupStalePaths()
				if err != nil {
					klog.Warning(err)
				}
				lastStaleCleanExecuted = time.Now()
			}
		}

		// some resting time
		time.Sleep(time.Second * 5)
	}

	// Reached here signifies the device was not found, throw an error
	return fmt.Errorf("fc device not found for wwn %s", d.WWN)
}

// tryCleaningFaultyAndOrphan: house keeping when device cannot be found once
func tryCleaningFaultyAndOrphan() {
	// handle faulty maps
	err := cleanupFaultyPaths()
	if err != nil {
		klog.Warning(err)
	}
	// handle orphan paths
	err = cleanupOrphanPaths()
	if err != nil {
		klog.Warning(err)
	}
	// handle stale maps
	err = cleanupStaleMaps()
	if err != nil {
		klog.Warning(err)
	}
	// handle error mappers
	err = cleanupErrorMultipathMaps()
	if err != nil {
		klog.Warning(err)
	}
}

// scsiHostRescan: scans all scsi hosts
func scsiHostRescan() error {
	scsiPath := "/sys/class/scsi_host/"
	if dirs, err := os.ReadDir(scsiPath); err == nil {
		for _, f := range dirs {
			name := scsiPath + f.Name() + "/scan"
			data := []byte("- - -")
			err := os.WriteFile(name, data, 0666)
			if err != nil {
				return fmt.Errorf("scsi host rescan failed: %v", err)
			}
		}
	}
	return nil
}

func ReadData(devPath string) (bool, *StagingDevice) {
	isFileExist, _, _ := fileExists(devPath, deviceInfoFileName)
	if isFileExist {
		stgDev, err := readData(devPath, deviceInfoFileName)
		klog.Warning(err)
		return true, stgDev
	}
	return false, nil
}

func WriteData(devPath string, stgDev *StagingDevice) error {
	if stgDev == nil {
		return fileDelete(devPath, deviceInfoFileName)
	}
	return writeData(devPath, deviceInfoFileName, stgDev)
}
