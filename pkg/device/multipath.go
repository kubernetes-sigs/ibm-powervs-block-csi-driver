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

	"k8s.io/klog"
)

const (
	multipathd         = "multipathd"
	dmsetupcommand     = "dmsetup"
	majorMinorPattern  = "(.*)\\((?P<Major>\\d+),\\s+(?P<Minor>\\d+)\\)"
	orphanPathsPattern = ".*\\s+(?P<host>\\d+):(?P<channel>\\d+):(?P<target>\\d+):(?P<lun>\\d+).*orphan"
	faultyPathsPattern = ".*failed.*(?P<host>\\d+):(?P<channel>\\d+):(?P<target>\\d+):(?P<lun>\\d+).*faulty"
	errorMapPattern    = "((?P<mapname>.*):.*error)"
	deviceDoesNotExist = "No such device or address"
)

var (
	showPathsFormat  = []string{"show", "paths", "format", "%w %d %t %i %o %T %z %s %m"}
	showMapsFormat   = []string{"show", "maps", "format", "%w %d %n %s"}
	errorMapRegex    = regexp.MustCompile(errorMapPattern)
	orphanPathRegexp = regexp.MustCompile(orphanPathsPattern)
	faultyPathRegexp = regexp.MustCompile(faultyPathsPattern)
	multipathMutex   sync.Mutex
)

// PathInfo : struct for multipathd show paths
type PathInfo struct {
	UUID     string
	Device   string
	DmState  string
	Hcil     string
	DevState string
	ChkState string
	Checker  string
}

func retryGetPathOfDevice(dev *Device, needActivePath bool) (paths []*PathInfo, err error) {
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
		if needActivePath {
			if len(paths) == 0 {
				// don't treat no scsi paths as an error
				return paths, nil
			}
		}
		return paths, nil
	}
}

// multipathGetPathsOfDevice : get all scsi paths and host, channel information of multipath device
func multipathGetPathsOfDevice(dev *Device, needActivePath bool) (paths []*PathInfo, err error) {
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
				paths = append(paths, path)
				// return only active paths with chk_st as ready and dm_st as active
			} else if len(entry) >= 5 && entry[2] == "active" && entry[5] == "ready" {
				path := &PathInfo{
					UUID:     entry[0][1:], // entry[0] uuid
					Device:   entry[1],     // entry[1] dev
					DmState:  entry[2],     // entry[2] dm_st
					Hcil:     entry[3],     // entry[3] hcil
					ChkState: entry[5],     // entry[5] chk_st
				}
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
}

func multipathdShowCmd(wwid string, args []string) (output []string, err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	out, err := exec.Command(multipathd, args...).CombinedOutput()
	if err != nil {
		return nil, err
	}
	// rc can be 0 on the below error conditions as well
	if isMultipathTimeoutError(string(out)) {
		err = fmt.Errorf("failed to get multipathd %v, out %s", args, out)
		return nil, err
	}
	r, err := regexp.Compile("(?m)^.*" + wwid + ".*$")
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

// tearDownMultipathDevice : tear down the hiearchy of multipath device and scsi paths
func tearDownMultipathDevice(dev *Device) (err error) {
	lines, err := multipathdShowCmd(dev.WWID, showMapsFormat)
	if err != nil {
		return err
	}
	for _, line := range lines {
		if strings.Contains(line, dev.WWID) {
			entry := strings.Fields(line)
			if strings.Contains(entry[0], dev.WWID) {
				err := retryCleanupDeviceAndSlaves(dev)
				if err != nil {
					klog.Warningf("error while deleting multipath device %s: %v", dev.Mapper, err)
					return err
				}
			}
		}
	}
	return nil
}

// retry for maxtries for device Cleanup
func retryCleanupDeviceAndSlaves(dev *Device) error {
	try := 0
	maxTries := 10 // retry for 50 seconds with periodic interval of 5 seconds
	for {
		err := cleanupDeviceAndSlaves(dev)
		if err != nil {
			if try < maxTries {
				try++
				time.Sleep(5 * time.Second)
				continue
			}
			return err
		}
		return nil
	}
}

// cleanupDeviceAndSlaves : remove the multipath devices and its slaves and logout iscsi targets
func cleanupDeviceAndSlaves(dev *Device) (err error) {
	// check for all paths again and make sure we have all paths for cleanup
	allPaths, err := multipathGetPathsOfDevice(dev, false)
	if err != nil {
		return err
	}

	// disable queuing on multipath
	_ = multipathDisableQueuing(dev)

	// remove dm device
	removeErr := multipathRemoveDmDevice(dev)

	// delete scsi devices
	err = deleteSdDevices(allPaths)
	if err != nil {
		return err
	}

	// indicate if we were not able to cleanup multipath map, so the caller can retry
	if removeErr != nil {
		return removeErr
	}
	return nil
}

// multipathDisableQueuing : disable queueing on the multipath device
func multipathDisableQueuing(dev *Device) (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

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

// multipathRemoveDmDevice : remove multipath device ps via dmsetup
func multipathRemoveDmDevice(dev *Device) (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

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

func deleteSdDevices(paths []*PathInfo) error {
	for _, path := range paths {
		err := deleteSdDevice(path.Device)
		if err != nil {
			// try to cleanup rest of the paths
			klog.Warningf("error while deleting device %s: %v", path, err)
			continue
		}
	}
	return nil
}

// deleteSdDevice : delete the scsi device
func deleteSdDevice(path string) (err error) {
	//deletePath for deleting the device
	deletePath := fmt.Sprintf("/sys/block/%s/device/delete", path)
	is, _, _ := FileExists(deletePath)
	if !is {
		// path seems to be already cleaned up so we return success
		// err = fmt.Errorf("path %s doesn't exist", deletePath)
		return nil
	}
	err = os.WriteFile(deletePath, []byte("1\n"), 0644)
	if err != nil {
		err = fmt.Errorf("error writing to file %s : %v", deletePath, err)
		return err
	}
	return nil
}

func cleanupErrorMultipathMaps() (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	// run dmsetup table and fetch error maps
	args := []string{"table"}
	outBytes, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
	if err != nil {
		return err
	}
	out := string(outBytes)
	listErrorMaps := errorMapRegex.FindAllString(out, -1)
	for _, errorMap := range listErrorMaps {
		result := findStringSubmatchMap(errorMap, errorMapRegex)
		if mapName, ok := result["mapname"]; ok {
			args := []string{"remove", "--force", mapName}
			removeOut, err := exec.Command(dmsetupcommand, args...).CombinedOutput()
			if err != nil {
				// if device or resource busy
				// simply try umount and then remove (best effort run only)
				if isMultipathBusyError(string(removeOut)) {
					umountArgs := []string{fmt.Sprintf("/dev/mapper/%s", mapName)}
					exec.Command("umount", umountArgs...).Run()
					exec.Command(dmsetupcommand, args...).Run()
				}
				// ignore errors as its a best effort to cleanup all error maps
				continue
			}
		}
	}
	return nil
}

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
		err := deleteSdDeviceByHctl(result["host"], result["channel"], result["target"], result["lun"])
		if err != nil {
			// ignore errors as its a best effort to cleanup all orphan maps
			continue
		}
	}
	return nil
}

// nolint as we want to keep updatePathSerialByHctl separate
func deleteSdDeviceByHctl(h string, c string, t string, l string) (err error) {
	deletePath := fmt.Sprintf("/sys/class/scsi_device/%s:%s:%s:%s/device/delete", h, c, t, l)
	is, _, _ := FileExists(deletePath)
	if is {
		err := os.WriteFile(deletePath, []byte("1"), 0644)
		if err != nil {
			err = fmt.Errorf("error writing to file %s : %v", deletePath, err)
			return err
		}
	}
	return nil
}

// cleanupStaleMaps cleanup maps which are not attached to a vend/prod/rev
func cleanupStaleMaps() (err error) {
	multipathMutex.Lock()
	defer multipathMutex.Unlock()

	klog.Info("inside cleanupStaleMaps")
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
	klog.Infof("inside cleanupStaleMaps: found %d stale paths", len(staleMultipaths))
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
		err := deleteSdDeviceByHctl(result["host"], result["channel"], result["target"], result["lun"])
		if err != nil {
			// ignore errors as its a best effort to cleanup all fulty paths
			continue
		}
	}
	return nil
}
