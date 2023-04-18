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
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// findStringSubmatchMap : find and build  the map of named groups
func findStringSubmatchMap(s string, r *regexp.Regexp) map[string]string {
	captures := make(map[string]string)
	match := r.FindStringSubmatch(s)
	if match == nil {
		return captures
	}
	for i, name := range r.SubexpNames() {
		if i != 0 {
			captures[name] = match[i]
		}
	}
	return captures
}

func readFirstLine(filePath string) (line string, er error) {
	file, err := os.Open(filePath)
	a := ""
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		a = scanner.Text()
		break
	}
	if err = scanner.Err(); err != nil {
		return "", err
	}
	return a, err
}

// FileExists does a stat on the path and returns true if it exists
// In addition, dir returns true if the path is a directory
func FileExists(path string) (exists bool, dir bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}
		return true, false, err
	}
	return true, info.IsDir(), nil
}

// WriteData persists data as json file at the provided location. Creates new directory if not already present.
func WriteData(dir string, fileName string, data interface{}) error {
	dataFilePath := filepath.Join(dir, fileName)
	// Encode from json object
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// Attempt create of staging dir, as CSI attacher can remove the directory
	// while operation is still pending(during retries)
	if err = os.MkdirAll(dir, 0750); err != nil {
		return err
	}

	// Write to file
	err = os.WriteFile(dataFilePath, jsonData, 0600)
	if err != nil {
		return err
	}
	return nil
}

func ReadData(dir, fileName string) ([]byte, error) {
	// Check if the file exists
	dataFilePath := filepath.Join(dir, fileName)
	exists, _, err := FileExists(dataFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to check if device info file %s exists, %v", dataFilePath, err.Error())
	}
	if !exists {
		return nil, fmt.Errorf("Device info file %s does not exist", dataFilePath)
	}

	// Read from file
	deviceInfo, err := os.ReadFile(dataFilePath)
	if err != nil {
		return nil, err
	}
	return deviceInfo, nil

}

func ReadStagedDeviceInfo(dir, fileName string) (*StagingDevice, error) {
	deviceInfo, err := ReadData(dir, fileName)
	if err != nil {
		return nil, err
	}

	// Decode into device object
	var stagingDev StagingDevice
	err = json.Unmarshal(deviceInfo, &stagingDev)
	if err != nil {
		return nil, err
	}

	return &stagingDev, nil
}

// FileDelete : delete the file
func FileDelete(path string) error {
	is, _, _ := FileExists(path)
	if !is {
		return errors.New("File doesnt exist " + path)
	}
	err := os.RemoveAll(path)
	if err != nil {
		return errors.New("Unable to delete file " + path + " " + err.Error())
	}
	return nil
}
