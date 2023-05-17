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
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

var (
	lastCleanExecuted      time.Time
	lastStaleCleanExecuted time.Time
)

type LinuxDevice interface {
	// GetDevice() bool
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

// NewLinuxDevice: new device with given wwn
func NewLinuxDevice(wwn string) (bool, LinuxDevice) {
	d := &Device{
		WWN: wwn,
	}
	err := d.getLinuxDmDevice(false)
	if err != nil || d.Mapper == "" {
		return false, d
	}

	return true, d
}

func (d *Device) GetMapper() string {
	return d.Mapper
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

		if !strings.EqualFold(d.WWN, tmpWWN) {
			continue
		}

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
		for _, path := range multipathdShowPaths {
			if !needActivePath || path.ChkState == "ready" {
				d.Slaves = append(d.Slaves, path.Device)
			}
		}
	}

	return nil
}

// DeleteDevice: delete the multipath device
func (d *Device) DeleteDevice() (err error) {
	if err = tearDownMultipathDevice(d); err != nil {
		return err
	}
	d.Mapper = ""
	d.Slaves = nil
	return nil
}

// CreateDevice: attach and create linux devices to host
func (d *Device) CreateDevice() (err error) {
	if err = scsiHostRescan(); err != nil {
		return err
	}
	// wait for device to appear after rescan
	time.Sleep(time.Second * 1)

	if err = d.createLinuxDevice(); err != nil {
		klog.Errorf("unable to create device for wwn %v", d.WWN)
		return err
	}

	if d.Mapper == "" {
		return fmt.Errorf("unable to find the device for wwn %s", d.WWN)
	}
	return nil
}

// createLinuxDevice: attaches and creates a new linux device
// Try device discovery; retry every 5 sec if no device found or have 0 slaves
// In between checks we will try to cleanup:
// a. faulty and orphan paths (quick)
// b. stale paths (time consuming)
// Since there can be parallel requests for device discovery we are not
// allowing cleanup process to run for each everytime.
// For 'a' we will not cleanup if it was already run within last 10 secs.
// For 'b' we will not cleanup if it was already run within last 25 secs.
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
	operations := []struct {
		cleanupFunc func() error
		description string
	}{
		{cleanupFunc: cleanupFaultyPaths, description: "faulty paths"},
		{cleanupFunc: cleanupOrphanPaths, description: "orphan paths"},
		{cleanupFunc: cleanupStaleMaps, description: "stale maps"},
		{cleanupFunc: cleanupErrorMultipathMaps, description: "error mappers"},
	}
	for _, op := range operations {
		if err := op.cleanupFunc(); err != nil {
			klog.Warningf("Failed to cleanup %s: %v", op.description, err)
		}
	}
}

// scsiHostRescan: scans all scsi hosts
func scsiHostRescan() error {
	scsiPath := "/sys/class/scsi_host/"
	dirs, err := os.ReadDir(scsiPath)
	if err != nil {
		return err
	}
	for _, f := range dirs {
		name := scsiPath + f.Name() + "/scan"
		data := []byte("- - -")
		if err := os.WriteFile(name, data, 0666); err != nil {
			return fmt.Errorf("scsi host rescan failed: %v", err)
		}
	}

	return nil
}

func GetDeviceWWN(pathName string) (wwn string, err error) {
	if strings.HasPrefix(pathName, "/dev/mapper/") {
		// get dm path
		pathName, err = filepath.EvalSymlinks(pathName)
		if err != nil {
			return "", err
		}
	}
	pathName = strings.TrimPrefix(pathName, "/dev/")

	uuid, err := getUUID(pathName)

	tmpWWID := strings.TrimPrefix(uuid, "mpath-")
	wwn = tmpWWID[1:] // truncate scsi-id prefix

	return wwn, err
}
