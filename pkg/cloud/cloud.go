/*
Copyright 2019 The Kubernetes Authors.

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

package cloud

import (
	"errors"

	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/util"
)

// PowerVS volume types.
// More information: https://cloud.ibm.com/docs/power-iaas?topic=power-iaas-on-cloud-architecture#storage-tiers
const (
	VolumeTypeTier0  = "tier0"  // 25 IOPS/GB
	VolumeTypeTier1  = "tier1"  // 10 IOPS/GB
	VolumeTypeTier3  = "tier3"  // 3 IOPS/GB
	VolumeTypeTier5k = "tier5k" // 5000 IOPS regardless of size
)

var (
	ValidVolumeTypes = []string{
		VolumeTypeTier1,
		VolumeTypeTier3,
		VolumeTypeTier5k,
		VolumeTypeTier0,
	}
)

// Defaults.
const (
	// DefaultVolumeSize represents the default volume size.
	DefaultVolumeSize int64 = 10 * util.GiB
	// DefaultVolumeType specifies which storage to use for newly created Volumes.
	DefaultVolumeType = VolumeTypeTier1
)

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = errors.New("resource was not found")

	// ErrAlreadyExists is returned when a resource is already existent.
	ErrAlreadyExists = errors.New("resource already exists")

	ErrVolumeNameAlreadyExists = errors.New("volume name already exists for cloud instance")

	ErrVolumeNotFound = errors.New("volume not found")

	ErrConflictVolumeAlreadyExists = errors.New("Conflict: unable to attach volumes to the server")

	ErrBadRequestVolumeNotFound = errors.New("Bad Request: the following volumes do not exist")

	ErrPVInstanceNotFound = errors.New("pvm-instance not found")

	ErrVolumeDetachNotFound = errors.New("volume does not exist")
)

// Disk represents a PowerVS volume.
type Disk struct {
	VolumeID    string
	DiskType    string
	WWN         string
	Name        string
	Shareable   bool
	CapacityGiB int64
	State       string
}

// DiskOptions represents parameters to create an PowerVS volume.
type DiskOptions struct {
	// PowerVS options
	Shareable bool
	// CapacityGigaBytes float64
	CapacityBytes int64
	VolumeType    string
}
