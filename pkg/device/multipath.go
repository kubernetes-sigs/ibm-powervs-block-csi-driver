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
	"sync"
	"time"

	"k8s.io/klog/v2"
)

const (
	multipathd           = "multipathd"
	dmsetupcommand       = "dmsetup"
	majorMinorPattern    = "(.*)\\((?P<Major>\\d+),\\s+(?P<Minor>\\d+)\\)"
	orphanPathsPattern   = ".*\\s+(?P<host>\\d+):(?P<channel>\\d+):(?P<target>\\d+):(?P<lun>\\d+).*orphan"
	faultyPathsPattern   = ".*failed.*(?P<host>\\d+):(?P<channel>\\d+):(?P<target>\\d+):(?P<lun>\\d+).*faulty"
	errorMapPattern      = "((?P<mapname>.*):.*error)"
	deviceDoesNotExist   = "No such device or address"
	scsiDeviceDeletePath = "/sys/class/scsi_device/%s:%s:%s:%s/device/delete"
)

var (
	showPathsFormat  = []string{"show", "paths", "raw", "format", "%w %d %t %i %o %T %z %s %m"}
	showMapsFormat   = []string{"show", "maps", "raw", "format", "%w %d %n %s"}
	errorMapRegex    = regexp.MustCompile(errorMapPattern)
	orphanPathRegexp = regexp.MustCompile(orphanPathsPattern)
	faultyPathRegexp = regexp.MustCompile(faultyPathsPattern)
	multipathMutex   sync.Mutex
)

// PathInfo: struct for multipathd show paths
type PathInfo struct {
	UUID     string
	Device   string
	DmState  string
	Hcil     string
	DevState string
	ChkState string
	Checker  string
}

// retryGetPathOfDevice: get all slaves for a given device
func retryGetPathOfDevice(dev *Device, needActivePath bool) (paths []PathInfo, err error) {
	try := 0
	maxTries := 5
	for {
		paths, err := multipathGetPathsOfDevice(dev, needActivePath)
		if err != nil {
			if isMultipathTimeoutError(err.Error()) {
				if try < maxTries {
					try++
					time.Sleep(5 * time.Second)
					continue
				}
			}
			return nil, err
		}
		return paths, nil
	}
}

// multipathGetPathsOfDevice: get all scsi paths and host, channel information of multipath device
func multipathGetPathsOfDevice(dev *Device, needActivePath bool) (paths []PathInfo, err error) {
	lines, err := multipathdShowCmd(dev.WWID, showPathsFormat)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 && dev != nil && dev.WWID != "" {
		return nil, nil
	}

	for _, line := range lines {
		if strings.Contains(line, dev.WWID) {
			entry := strings.Fields(line)
			// return all paths if we don't need active paths
			if !needActivePath {
				path := &PathInfo{
					UUID:     entry[0][1:], // entry[0] uuid
					Device:   entry[1],     // entry[1] dev
					DmState:  entry[2],     // entry[2] dm_st
					Hcil:     entry[3],     // entry[3] hcil
					ChkState: entry[5],     // entry[5] chk_st
				}
				paths = append(paths, *path)
				// return only active paths with chk_st as ready and dm_st as active
			} else if len(entry) >= 5 && entry[2] == "active" && entry[5] == "ready" {
				path := &PathInfo{
					UUID:     entry[0][1:], // entry[0] uuid
					Device:   entry[1],     // entry[1] dev
					DmState:  entry[2],     // entry[2] dm_st
					Hcil:     entry[3],     // entry[3] hcil
					ChkState: entry[5],     // entry[5] chk_st
				}
				paths = append(paths, *path)
			}
		}
	}
	return paths, nil
}

// multipathdShowCmd: runs multipathd show ... with given arguments
func multipathdShowCmd(search string, args []string) (output []string, err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	out, err := exec.Command(multipathd, args...).CombinedOutput()
	if err != nil {
		return nil, err
	}
	r, err := regexp.Compile("(?m)^.*" + search + ".*$")
	if err != nil {
		return nil, err
	}
	listMultipathShowCmdOut := r.FindAllString(string(out), -1)
	return listMultipathShowCmdOut, nil
}

func isMultipathTimeoutError(msg string) bool {
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "receiving packet")
}
func isMultipathBusyError(msg string) bool {
	return strings.Contains(msg, "Device or resource busy")
}

// tearDownMultipathDevice: tear down the hiearchy of multipath device and scsi paths
func tearDownMultipathDevice(dev *Device) (err error) {
	lines, err := multipathdShowCmd(dev.WWID, showMapsFormat)
	if err != nil {
		return err
	}

	for _, line := range lines {
		if !strings.Contains(line, dev.WWID) {
			continue
		}
		entry := strings.Fields(line)
		if len(entry) == 0 || !strings.Contains(entry[0], dev.WWID) {
			continue
		}
		if err := retryCleanupDeviceAndSlaves(dev); err != nil {
			klog.Warningf("error while deleting multipath device %s: %v", dev.Mapper, err)
			return err
		}
	}
	return nil
}

// retryCleanupDeviceAndSlaves: retry for maxtries for device Cleanup
func retryCleanupDeviceAndSlaves(dev *Device) (err error) {
	maxTries := 10
	for try := 0; try < maxTries; try++ {
		err = cleanupDeviceAndSlaves(dev)
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

// cleanupDeviceAndSlaves: remove the multipath devices and its slaves
func cleanupDeviceAndSlaves(dev *Device) (err error) {
	// check for all paths again and make sure we have all paths for cleanup
	allPaths, err := multipathGetPathsOfDevice(dev, false)
	if err != nil {
		return err
	}

	removeErr := multipathRemoveDmDevice(dev)
	// delay returning removeErr so we can attempt to delete all the scsi devices
	// this indicates multipath map cleanup failed, so the caller can retry

	// delete scsi devices (slaves)
	for _, path := range allPaths {
		deletePath := fmt.Sprintf("/sys/block/%s/device/delete", path.Device)
		if err := deleteSdDevice(deletePath); err != nil {
			klog.Warningf("error while deleting device %s: %v", path, err)
			// try to cleanup rest of the paths
		}
	}

	return removeErr
}

// multipathDisableQueuing: disable queueing on the multipath device
func multipathDisableQueuing(dev *Device) (err error) {
	args := []string{"message", dev.Mapper, "0", "fail_if_no_path"}
	outBytes, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
	if err != nil {
		return err
	}
	out := string(outBytes)
	if out != "" && strings.Contains(out, deviceDoesNotExist) {
		return fmt.Errorf("failed to disable queuing for %s with wwn %s. Error: %s", dev.Mapper, dev.WWN, out)
	}
	return nil
}

// multipathRemoveDmDevice: remove multipath device via dmsetup
func multipathRemoveDmDevice(dev *Device) (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	_ = multipathDisableQueuing(dev)

	args := []string{"remove", "--force", dev.Mapper}
	outBytes, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove multipath map for %s with wwn %s. Error: %v", dev.Mapper, dev.WWN, err)
	}
	out := string(outBytes)
	if out != "" && !strings.Contains(out, "ok") && !strings.Contains(out, deviceDoesNotExist) {
		return fmt.Errorf("failed to remove device map for %s with wwn %s. Error: %s", dev.Mapper, dev.WWN, out)
	}
	return nil
}

// cleanupErrorMultipathMaps: find error maps and remove them
func cleanupErrorMultipathMaps() (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	// run dmsetup table and fetch error maps
	args := []string{"table"}
	outBytes, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
	if err != nil {
		return err
	}

	listErrorMaps := errorMapRegex.FindAllString(string(outBytes), -1)
	for _, errorMap := range listErrorMaps {
		result := findStringSubmatchMap(errorMap, errorMapRegex)
		if mapName, ok := result["mapname"]; ok {
			args := []string{"remove", "--force", mapName}
			if removeOut, err := exec.Command(dmsetupcommand, args...).CombinedOutput(); err == nil {
				continue
			} else if isMultipathBusyError(string(removeOut)) {
				// if device or resource busy
				// simply try umount and then remove (best effort run only)
				umountArgs := []string{fmt.Sprintf("/dev/mapper/%s", mapName)}
				_ = exec.Command("umount", umountArgs...).Run()
				_ = exec.Command(dmsetupcommand, args...).Run()
			}
		}
	}
	return nil
}

// cleanupOrphanPaths: find orphan paths and remove them
func cleanupOrphanPaths() (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	// run multipathd show paths and fetch orphan maps
	outBytes, err := exec.Command(multipathd, showPathsFormat...).CombinedOutput()
	if err != nil {
		return err
	}
	out := string(outBytes)
	// rc can be 0 on the below error conditions as well
	if isMultipathTimeoutError(out) {
		err = fmt.Errorf("failed to get multipathd %v, out %s", showPathsFormat, out)
		return err
	}

	listOrphanPaths := orphanPathRegexp.FindAllString(out, -1)
	for _, orphanPath := range listOrphanPaths {
		result := findStringSubmatchMap(orphanPath, orphanPathRegexp)
		deletePath := fmt.Sprintf(scsiDeviceDeletePath, result["host"], result["channel"], result["target"], result["lun"])
		if err := deleteSdDevice(deletePath); err != nil {
			// ignore errors as its a best effort to cleanup all orphan maps
			klog.Warningf("error while deleting device: %v", err)
		}
	}
	return nil
}

// cleanupStaleMaps: cleanup maps which are not attached to a vend/prod/rev
func cleanupStaleMaps() (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	outBytes, err := exec.Command(multipathd, showMapsFormat...).CombinedOutput()
	if err != nil {
		return err
	}
	out := string(outBytes)
	// rc can be 0 on the below error conditions as well
	if isMultipathTimeoutError(out) {
		err = fmt.Errorf("failed to get multipathd %v, out %s", showMapsFormat, out)
		return err
	}
	r, err := regexp.Compile("(?m)^.*##,##")
	if err != nil {
		return err
	}
	staleMultipaths := r.FindAllString(out, -1)

	for _, line := range staleMultipaths {
		entry := strings.Fields(line)
		args := []string{"remove", "--force", entry[2]} // entry[2]: map name
		_, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
		if err != nil {
			continue
		}
	}
	return nil
}

// cleanupFaultyPaths: cleanup paths which are faulty, indicates no storage
func cleanupFaultyPaths() (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	// run multipathd show paths and fetch orphan maps
	outBytes, err := exec.Command(multipathd, showPathsFormat...).CombinedOutput()
	if err != nil {
		return err
	}
	out := string(outBytes)
	// rc can be 0 on the below error conditions as well
	if isMultipathTimeoutError(out) {
		err = fmt.Errorf("failed to get multipathd %v, out %s", showPathsFormat, out)
		return err
	}

	listFaultyPaths := faultyPathRegexp.FindAllString(out, -1)
	for _, faultyPath := range listFaultyPaths {
		result := findStringSubmatchMap(faultyPath, faultyPathRegexp)
		deletePath := fmt.Sprintf(scsiDeviceDeletePath, result["host"], result["channel"], result["target"], result["lun"])
		if err := deleteSdDevice(deletePath); err != nil {
			// ignore errors as its a best effort to cleanup all fulty paths
			klog.Warningf("error while deleting device: %v", err)
		}
	}
	return nil
}

// cleanupStalePaths: clean stale scsi devices which can be result of remapped disks
func cleanupStalePaths() (err error) {
	// get all maps (.*mpath.*) to clean stale paths in `/sys/class/scsi_device`
	allMaps, err := multipathdShowCmd("mpath", showMapsFormat)
	if err != nil {
		klog.Info(err)
		return err
	}

	scsiPath := "/sys/class/scsi_device/"
	dirs, err := os.ReadDir(scsiPath)
	if err != nil {
		return err
	}
OUTER:
	for _, f := range dirs {
		wwidPath := scsiPath + f.Name() + "/device/wwid"
		wwid, err := readFirstLine(wwidPath)
		if err != nil {
			klog.Warning(err)
			continue
		}
		entries := strings.Split(wwid, ".")
		if len(entries) <= 1 {
			continue
		}
		wwn := entries[1]
		for _, line := range allMaps {
			if strings.Contains(line, wwn) {
				continue OUTER
			}
		}

		// delete stale path if not found
		hctl := strings.Split(f.Name(), ":")
		deletePath := fmt.Sprintf(scsiDeviceDeletePath, hctl[0], hctl[1], hctl[2], hctl[3])
		if err := deleteSdDevice(deletePath); err != nil {
			// ignore errors as its a best effort to cleanup all stale paths
			klog.Warningf("error while deleting device: %v", err)
		}
	}
	return nil
}
