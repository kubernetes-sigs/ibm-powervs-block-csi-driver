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
	"github.com/IBM-Cloud/power-go-client/errors"
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/client/p_cloud_volumes"
	"github.com/IBM-Cloud/power-go-client/power/models"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
	"github.com/davecgh/go-spew/spew"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/util"
)

var _ Cloud = &powerVSCloud{}

var (
	// TODO: Never seen these catalog values changing, lets add code to get these values in future for the sake of better design.

	// This can be found in the IBM Cloud catalog, command used to get this information is
	// $ ibmcloud catalog service-marketplace| grep power-iaas
	// abd259f0-9990-11e8-acc8-b9f54a8f1661 power-iaas
	powerVSServiceID = "abd259f0-9990-11e8-acc8-b9f54a8f1661"

	// PlanID can be fetched via command:
	// $ ibmcloud catalog service abd259f0-9990-11e8-acc8-b9f54a8f1661 | grep plan
	//                 power-virtual-server-group                    plan         f165dd34-3a40-423b-9d95-e90a23f724dd
	powerVSPlanID = "f165dd34-3a40-423b-9d95-e90a23f724dd"
)

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
	piSession *ibmpisession.IBMPISession

	cloudInstanceID string

	imageClient        *instance.IBMPIImageClient
	pvmInstancesClient *instance.IBMPIInstanceClient
	volClient          *instance.IBMPIVolumeClient
}

type PVMInstance struct {
	ID      string
	ImageID string
	Name    string
}

type PVMImage struct {
	ID       string
	Name     string
	DiskType string
}

func NewPowerVSCloud(cloudInstanceID, zone string, debug bool) (Cloud, error) {
	return newPowerVSCloud(cloudInstanceID, zone, debug)
}

func newPowerVSCloud(cloudInstanceID, zone string, debug bool) (Cloud, error) {
	apikey := os.Getenv("IBMCLOUD_API_KEY")

	serviceClientOptions := &resourcecontrollerv2.ResourceControllerV2Options{
		Authenticator: &core.IamAuthenticator{ApiKey: apikey},
	}
	serviceClient, err := resourcecontrollerv2.NewResourceControllerV2UsingExternalConfig(serviceClientOptions)
	if err != nil {
		return nil, fmt.Errorf("errored while creating NewResourceControllerV2UsingExternalConfig: %v", err)
	}
	resourceInstanceList, _, err := serviceClient.ListResourceInstances(&resourcecontrollerv2.ListResourceInstancesOptions{
		GUID:           &cloudInstanceID,
		ResourceID:     &powerVSServiceID,
		ResourcePlanID: &powerVSPlanID,
	})
	if err != nil {
		return nil, fmt.Errorf("errored while listing the Power VS service instance with ID: %s, err: %v", cloudInstanceID, err)
	}

	if len(resourceInstanceList.Resources) == 0 {
		return nil, fmt.Errorf("no Power VS service instance found with ID: %s", cloudInstanceID)
	}

	authenticator := &core.IamAuthenticator{ApiKey: apikey}
	piOptions := ibmpisession.IBMPIOptions{Authenticator: authenticator, Debug: debug, UserAccount: *resourceInstanceList.Resources[0].AccountID, Zone: zone}
	piSession, err := ibmpisession.NewIBMPISession(&piOptions)
	if err != nil {
		return nil, err
	}

	backgroundContext := context.Background()
	volClient := instance.NewIBMPIVolumeClient(backgroundContext, piSession, cloudInstanceID)
	pvmInstancesClient := instance.NewIBMPIInstanceClient(backgroundContext, piSession, cloudInstanceID)
	imageClient := instance.NewIBMPIImageClient(backgroundContext, piSession, cloudInstanceID)

	return &powerVSCloud{
		piSession:          piSession,
		cloudInstanceID:    cloudInstanceID,
		imageClient:        imageClient,
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
				ID:      *pvmInstance.PvmInstanceID,
				ImageID: *pvmInstance.ImageID,
				Name:    *pvmInstance.ServerName,
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
		ID:      *in.PvmInstanceID,
		ImageID: *in.ImageID,
		Name:    *in.ServerName,
	}, nil
}

func (p *powerVSCloud) GetImageByID(imageID string) (*PVMImage, error) {
	image, err := p.imageClient.Get(imageID)
	if err != nil {
		return nil, err
	}
	return &PVMImage{
		ID:       *image.ImageID,
		Name:     *image.Name,
		DiskType: *image.StorageType,
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
		Size:      pointer.Float64Ptr(float64(capacityGiB)),
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
	err := wait.PollImmediate(PollInterval, PollTimeout, func() (bool, error) {
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
	//TODO: remove capacityBytes
	params := p_cloud_volumes.NewPcloudCloudinstancesVolumesGetallParamsWithTimeout(TIMEOUT).WithCloudInstanceID(p.cloudInstanceID)
	resp, err := p.piSession.Power.PCloudVolumes.PcloudCloudinstancesVolumesGetall(params, p.piSession.AuthInfo(p.cloudInstanceID))
	if err != nil {
		return nil, errors.ToError(err)
	}
	for _, v := range resp.Payload.Volumes {
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
