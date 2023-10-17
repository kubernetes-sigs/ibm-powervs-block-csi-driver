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
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/IBM-Cloud/power-go-client/clients/instance"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/util"
)

var _ Cloud = &powerVSCloud{}

const (
	PollTimeout          = 120 * time.Second
	PollInterval         = 5 * time.Second
	TIMEOUT              = 60 * time.Minute
	VolumeInUseState     = "in-use"
	VolumeAvailableState = "available"
)

type PowerVSClient interface {
}

type powerVSCloud struct {
	pvmInstancesClient *instance.IBMPIInstanceClient
	volClient          *instance.IBMPIVolumeClient
}

type PVMInstance struct {
	ID       string
	DiskType string
	Name     string
}

func NewPowerVSCloud(cloudInstanceID, zone string, debug bool) (Cloud, error) {
	return newPowerVSCloud(cloudInstanceID, zone, debug)
}

func newPowerVSCloud(cloudInstanceID, zone string, debug bool) (Cloud, error) {
	apikey := os.Getenv("IBMCLOUD_API_KEY")

	authenticator := &core.IamAuthenticator{ApiKey: apikey}

	serviceClientOptions := &resourcecontrollerv2.ResourceControllerV2Options{
		Authenticator: authenticator,
	}
	serviceClient, err := resourcecontrollerv2.NewResourceControllerV2(serviceClientOptions)
	if err != nil {
		return nil, fmt.Errorf("errored while creating NewResourceControllerV2: %v", err)
	}
	resourceInstance, _, err := serviceClient.GetResourceInstance(&resourcecontrollerv2.GetResourceInstanceOptions{
		ID: &cloudInstanceID,
	})

	if err != nil {
		return nil, fmt.Errorf("errored while getting the Power VS service instance with ID: %s, err: %v", cloudInstanceID, err)
	}

	piOptions := ibmpisession.IBMPIOptions{Authenticator: authenticator, Debug: debug, UserAccount: *resourceInstance.AccountID, Zone: zone}
	piSession, err := ibmpisession.NewIBMPISession(&piOptions)
	if err != nil {
		return nil, err
	}

	backgroundContext := context.Background()
	volClient := instance.NewIBMPIVolumeClient(backgroundContext, piSession, cloudInstanceID)
	pvmInstancesClient := instance.NewIBMPIInstanceClient(backgroundContext, piSession, cloudInstanceID)

	return &powerVSCloud{
		pvmInstancesClient: pvmInstancesClient,
		volClient:          volClient,
	}, nil
}

func (p *powerVSCloud) GetPVMInstanceByName(name string) (*PVMInstance, error) {
	in, err := p.pvmInstancesClient.GetAll()
	if err != nil {
		return nil, err
	}
	for _, pvmInstance := range in.PvmInstances {
		if name == *pvmInstance.ServerName {
			return &PVMInstance{
				ID:       *pvmInstance.PvmInstanceID,
				DiskType: pvmInstance.StorageType,
				Name:     *pvmInstance.ServerName,
			}, nil
		}
	}
	return nil, ErrNotFound
}

func (p *powerVSCloud) GetPVMInstanceByID(instanceID string) (*PVMInstance, error) {
	in, err := p.pvmInstancesClient.Get(instanceID)
	if err != nil {
		return nil, err
	}

	return &PVMInstance{
		ID:       *in.PvmInstanceID,
		DiskType: *in.StorageType,
		Name:     *in.ServerName,
	}, nil
}

func (p *powerVSCloud) CreateDisk(volumeName string, diskOptions *DiskOptions) (disk *Disk, err error) {
	var volumeType string
	capacityGiB := util.BytesToGiB(diskOptions.CapacityBytes)

	switch diskOptions.VolumeType {
	case VolumeTypeTier1, VolumeTypeTier3:
		volumeType = diskOptions.VolumeType
	case "":
		volumeType = DefaultVolumeType
	default:
		return nil, fmt.Errorf("invalid PowerVS VolumeType %q", diskOptions.VolumeType)
	}

	dataVolume := &models.CreateDataVolume{
		Name:      &volumeName,
		Size:      ptr.To[float64](float64(capacityGiB)),
		Shareable: &diskOptions.Shareable,
		DiskType:  volumeType,
	}

	v, err := p.volClient.CreateVolume(dataVolume)
	if err != nil {
		return nil, err
	}

	err = p.WaitForVolumeState(*v.VolumeID, VolumeAvailableState)
	if err != nil {
		return nil, err
	}

	return &Disk{CapacityGiB: capacityGiB, VolumeID: *v.VolumeID, DiskType: v.DiskType, WWN: strings.ToLower(v.Wwn)}, nil
}

func (p *powerVSCloud) DeleteDisk(volumeID string) (success bool, err error) {
	err = p.volClient.DeleteVolume(volumeID)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (p *powerVSCloud) AttachDisk(volumeID string, nodeID string) (err error) {
	err = p.volClient.Attach(nodeID, volumeID)
	if err != nil {
		return err
	}

	err = p.WaitForVolumeState(volumeID, VolumeInUseState)
	if err != nil {
		return err
	}
	return nil
}

func (p *powerVSCloud) DetachDisk(volumeID string, nodeID string) (err error) {
	err = p.volClient.Detach(nodeID, volumeID)
	if err != nil {
		return err
	}
	err = p.WaitForVolumeState(volumeID, VolumeAvailableState)
	if err != nil {
		return err
	}
	return nil
}

func (p *powerVSCloud) IsAttached(volumeID string, nodeID string) (attached bool, err error) {
	_, err = p.volClient.CheckVolumeAttach(nodeID, volumeID)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (p *powerVSCloud) ResizeDisk(volumeID string, reqSize int64) (newSize int64, err error) {
	disk, err := p.GetDiskByID(volumeID)
	if err != nil {
		return 0, err
	}

	capacityGiB := util.BytesToGiB(reqSize)

	dataVolume := &models.UpdateVolume{
		Name:      &disk.Name,
		Size:      float64(capacityGiB),
		Shareable: &disk.Shareable,
	}

	v, err := p.volClient.UpdateVolume(volumeID, dataVolume)
	if err != nil {
		return 0, err
	}
	return int64(*v.Size), nil
}

func (p *powerVSCloud) WaitForVolumeState(volumeID, state string) error {
	ctx := context.Background()
	err := wait.PollUntilContextTimeout(ctx, PollInterval, PollTimeout, true, func(ctx context.Context) (bool, error) {
		v, err := p.volClient.Get(volumeID)
		if err != nil {
			return false, err
		}
		spew.Dump(v)
		return v.State == state, nil
	})
	if err != nil {
		return err
	}
	return nil
}

func (p *powerVSCloud) GetDiskByName(name string) (disk *Disk, err error) {
	vols, err := p.volClient.GetAll()
	if err != nil {
		return nil, err
	}
	for _, v := range vols.Volumes {
		if name == *v.Name {
			return &Disk{
				Name:        *v.Name,
				DiskType:    *v.DiskType,
				VolumeID:    *v.VolumeID,
				WWN:         strings.ToLower(*v.Wwn),
				Shareable:   *v.Shareable,
				CapacityGiB: int64(*v.Size),
			}, nil
		}
	}

	return nil, ErrNotFound
}

func (p *powerVSCloud) GetDiskByID(volumeID string) (disk *Disk, err error) {
	v, err := p.volClient.Get(volumeID)
	if err != nil {
		if strings.Contains(err.Error(), "Resource not found") {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &Disk{
		Name:        *v.Name,
		DiskType:    v.DiskType,
		VolumeID:    *v.VolumeID,
		WWN:         strings.ToLower(v.Wwn),
		Shareable:   *v.Shareable,
		CapacityGiB: int64(*v.Size),
	}, nil
}
