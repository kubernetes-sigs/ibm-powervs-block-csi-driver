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
	"strconv"
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
	Size     int64    `json:"size,omitempty"` // size in MiB
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

func GetDeviceFromVolume(wwn string) *Device {
	devices, err := getLinuxDmDevices(wwn, false)
	// ignore any error as this is before scan
	if err != nil || len(devices) == 0 {
		return nil
	}
	return devices[0]
}

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
			klog.V(5).Infof("getLinuxDmDevices FOUND for wwn %s", tmpWWN)
			dev.WWN = tmpWWN
			dev.WWID = tmpWWID
			devSize, err := getSizeOfDeviceInMiB(dev.Pathname)
			if err != nil {
				return nil, err
			}
			dev.Size = devSize
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

func getMpathName(pathname string) (string, error) {
	fileName := fmt.Sprintf("/sys/block/%s/dm/name", pathname)
	return readFirstLine(fileName)
}

func getUUID(pathname string) (string, error) {
	fileName := fmt.Sprintf("/sys/block/%s/dm/uuid", pathname)
	return readFirstLine(fileName)
}

func getSizeOfDeviceInMiB(pathname string) (int64, error) {
	fileName := fmt.Sprintf("/sys/block/%s/size", pathname)
	size, err := readFirstLine(fileName)
	if err != nil {
		err = fmt.Errorf("unable to get size for device: %s Err: %v", pathname, err)
		return -1, err
	}
	sizeInSector, err := strconv.ParseInt(size, 10, 0)
	if err != nil {
		err = fmt.Errorf("unable to parse size for device: %s Err: %v", pathname, err)
		return -1, err
	}
	return sizeInSector / 2 * 1024, nil
}

// DeleteDevice : delete the multipath device
func DeleteDevice(dev *Device) (err error) {
	try := 0
	maxTries := 3
	for {
		err := tearDownMultipathDevice(dev)
		if err != nil {
			if try < maxTries {
				try++
				time.Sleep(time.Duration(try) * time.Second)
				continue
			}
			return err
		}
		return nil
	}
}

// CreateDevice : attach and create linux devices to host
func CreateDevice(wwn string) (dev *Device, err error) {
	device, err := createLinuxDevice(wwn)
	if err != nil {
		klog.Errorf("unable to create device for wwn %v", wwn)
		// If we encounter an error, there may be some devices created and some not.
		// If we fail the operation , then we need to rollback all the created device
		//TODO : cleanup all the devices created
		return nil, err
	}

	return device, nil
}

// createLinuxDevice : attaches and creates a new linux device
func createLinuxDevice(wwn string) (dev *Device, err error) {
	err = scsiHostRescan()
	if err != nil {
		return nil, err
	}

	// sleeping for 1 second waiting for device %s to appear after rescan
	time.Sleep(time.Second * 1)

	// find multipath devices after the rescan and login
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
			klog.Info("running cleanup")
			tryCleaningFaultyAndOrphan()
			lastCleanExecuted = time.Now()
		}
		// search again before removing stale scsi disks
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

		// sleeping for 5 seconds waiting for device to appear after rescan
		time.Sleep(time.Second * 5)
	}

	// Reached here signifies the device was not found, throw an error
	return nil, fmt.Errorf("fc device not found for wwn %s", wwn)
}

func findDevice(wwn string) (*Device, error) {
	devices, err := getLinuxDmDevices(wwn, true)
	if err != nil {
		return nil, err
	}
	for _, d := range devices {
		if len(d.Slaves) > 0 && strings.EqualFold(d.WWN, wwn) {
			klog.V(5).Infof("createLinuxDevice FOUND for wwn %s and slaves %+v", d.WWN, d.Slaves)
			return d, nil
		}
	}
	return nil, nil
}
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

func scsiHostRescan() error {
	scsiPath := "/sys/class/scsi_host/"
	if dirs, err := os.ReadDir(scsiPath); err == nil {
		for _, f := range dirs {
			name := scsiPath + f.Name() + "/scan"
			data := []byte("- - -")
			err := os.WriteFile(name, data, 0666)
			if err != nil {
				return fmt.Errorf("scsi host rescan failed : error: %v", err)
			}
		}
	}
	return nil
}
