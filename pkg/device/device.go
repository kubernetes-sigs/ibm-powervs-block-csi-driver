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

// Device struct
type Device struct {
	Pathname string   `json:"pathname,omitempty"`
	Mapper   string   `json:"mapper,omitempty"`
	WWID     string   `json:"wwid,omitempty"`
	WWN      string   `json:"wwn,omitempty"`
	Slaves   []string `json:"slaves,omitempty"`
}

// StagingDevice represents the device information that is stored in the staging area.
type StagingDevice struct {
	VolumeID         string  `json:"volume_id"`
	VolumeAccessMode string  `json:"volume_access_mode"` // block or mount
	Device           *Device `json:"device"`
	MountInfo        *Mount  `json:"mount_info,omitempty"`
}

// Mount struct
type Mount struct {
	Mountpoint string   `json:"Mountpoint,omitempty"`
	Options    []string `json:"Options,omitempty"`
	Device     *Device  `json:"device,omitempty"`
	FSType     string   `json:"fstype,omitempty"`
}

// GetDevice: find the device with given wwn
func GetDevice(wwn string) *Device {
	devices, err := getLinuxDmDevices(wwn, false)
	// ignore any errors
	if err != nil || len(devices) == 0 {
		return nil
	}
	return devices[0]
}

// getLinuxDmDevices: get all linux Devices
func getLinuxDmDevices(wwn string, needActivePath bool) ([]*Device, error) {
	args := []string{"ls", "--target", "multipath"}
	outBytes, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
	out := string(outBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve multipath devices: %s", out)
	}

	var devices []*Device
	r, err := regexp.Compile(majorMinorPattern)
	if err != nil {
		return nil, fmt.Errorf("unable to compile regex with %s", majorMinorPattern)
	}
	listOut := r.FindAllString(out, -1)

	for _, line := range listOut {
		result := findStringSubmatchMap(line, r)
		dev := &Device{}
		dev.Pathname = "dm-" + result["Minor"]

		mapName, err := getMpathName(dev.Pathname)
		if err != nil {
			return nil, err
		}
		dev.Mapper = "/dev/mapper/" + mapName

		uuid, err := getUUID(dev.Pathname)
		if err != nil {
			return nil, err
		}
		tmpWWID := strings.TrimPrefix(uuid, "mpath-")
		tmpWWN := tmpWWID[1:] // truncate scsi-id prefix

		if strings.EqualFold(wwn, tmpWWN) {
			dev.WWN = tmpWWN
			dev.WWID = tmpWWID
			if err != nil {
				return nil, err
			}
			multipathdShowPaths, err := retryGetPathOfDevice(dev, needActivePath)
			if err != nil {
				err = fmt.Errorf("unable to get scsi slaves for device %s: %v", dev.WWN, err)
				return nil, err
			}
			var slaves []string
			for _, path := range multipathdShowPaths {
				if path != nil && (!needActivePath || path.ChkState == "ready") {
					slaves = append(slaves, path.Device)
				}
			}
			dev.Slaves = slaves
			if len(dev.Slaves) > 0 {
				devices = append(devices, dev)
			}

		}
	}

	return devices, nil
}

// DeleteDevice: delete the multipath device
func DeleteDevice(dev *Device) (err error) {
	return tearDownMultipathDevice(dev)
}

// CreateDevice: attach and create linux devices to host
func CreateDevice(wwn string) (dev *Device, err error) {
	err = scsiHostRescan()
	if err != nil {
		return nil, err
	}
	// wait for device to appear after rescan
	time.Sleep(time.Second * 1)

	device, err := createLinuxDevice(wwn)
	if err != nil {
		klog.Errorf("unable to create device for wwn %v", wwn)
		return nil, err
	}
	return device, nil
}

// createLinuxDevice: attaches and creates a new linux device
func createLinuxDevice(wwn string) (dev *Device, err error) {
	// Start a Countdown ticker
	for i := 0; i <= 8; i++ {
		// ignore devices with no paths
		// Match wwn
		dev, err := findDevice(wwn)
		if dev != nil || err != nil {
			return dev, err
		}
		// try cleaning up faulty, orphan, stale paths/maps
		tmpTime := time.Now().Add(-10 * time.Second)
		if lastCleanExecuted.Before(tmpTime) {
			tryCleaningFaultyAndOrphan()
			lastCleanExecuted = time.Now()
		}

		// search again before removing stale/remapped scsi disks
		dev, err = findDevice(wwn)
		if dev != nil || err != nil {
			return dev, err
		}

		// handle stale paths
		// heavy operation hence try only between 25 secs
		tmpTime = time.Now().Add(-25 * time.Second)
		if lastStaleCleanExecuted.Before(tmpTime) {
			err = cleanupStalePaths()
			if err != nil {
				klog.Warning(err)
			}
			lastStaleCleanExecuted = time.Now()
		}

		// some resting time
		time.Sleep(time.Second * 5)
	}

	// Reached here signifies the device was not found, throw an error
	return nil, fmt.Errorf("fc device not found for wwn %s", wwn)
}

// findDevice: find the device with given wwn and having atleast 1 slave disk
func findDevice(wwn string) (*Device, error) {
	devices, err := getLinuxDmDevices(wwn, true)
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if len(d.Slaves) > 0 && strings.EqualFold(d.WWN, wwn) {
			return d, nil
		}
	}
	return nil, nil
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