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

package driver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/gcfg.v1"

	"k8s.io/klog/v2"

	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/cloud"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/util"
)

// Supported access modes.
const (
	SingleNodeWriter     = csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER
	MultiNodeMultiWriter = csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER
	MultiNodeReaderOnly  = csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY
)

var (
	// controllerCaps represents the capability of controller service.
	controllerCaps = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
	}
)

// controllerService represents the controller service of CSI driver.
type controllerService struct {
	cloud         cloud.Cloud
	driverOptions *Options
	volumeLocks   *util.VolumeLocks
	csi.UnimplementedControllerServer
}

// Provider holds information from the cloud provider.
type Provider struct {
	// PowerVSCloudInstanceID is IBM Power VS service instance id
	PowerVSCloudInstanceID string `gcfg:"powerVSCloudInstanceID"`
	// PowerVSZone is IBM Power VS service zone
	PowerVSZone string `gcfg:"powerVSZone"`
}

// CloudConfig is the ibm cloud provider config data.
type CloudConfig struct {
	// [provider] section
	Prov Provider `gcfg:"provider"`
}

var (
	NewPowerVSCloudFunc = cloud.NewPowerVSCloud
)

// newControllerService creates a new controller service and prints stack trace and osexit if failed to create the service.
func newControllerService(driverOptions *Options) controllerService {
	var cloudInstanceId, zone string

	// Following env vars will be checked before looking up the
	// cli options and metadata service if both are not set.
	// It will help with running the controller tests locally
	// on a powervs vm. Currently, it relies on k8s cluster node.
	cID := os.Getenv("POWERVS_CLOUD_INSTANCE_ID")
	z := os.Getenv("POWERVS_ZONE")

	if cID != "" && z != "" {
		klog.V(4).Info("using node info from environment variables")
		cloudInstanceId, zone = cID, z
	} else if driverOptions.cloudconfig != "" {
		var cloudConfig CloudConfig
		config, err := os.Open(driverOptions.cloudconfig)
		if err != nil {
			klog.Fatalf("Failed to get cloud config: %v", err)
		}
		defer config.Close()

		if err := gcfg.FatalOnly(gcfg.ReadInto(&cloudConfig, config)); err != nil {
			klog.Fatalf("Failed to read cloud config: %v", err)
		}
		cloudInstanceId = cloudConfig.Prov.PowerVSCloudInstanceID
		zone = cloudConfig.Prov.PowerVSZone
		if cloudInstanceId == "" || zone == "" {
			klog.Fatalf("Failed to read cloud config: %v", status.Errorf(codes.NotFound, "cloud instance id or zone is empty"))
		}
	} else {
		klog.V(4).Infof("retrieving node info from metadata service")
		metadata, err := cloud.NewMetadataService(cloud.DefaultKubernetesAPIClient, driverOptions.kubeconfig)
		if err != nil {
			klog.Fatalf("Failed to get metadata service: %v", err)
		}
		cloudInstanceId = metadata.GetCloudInstanceId()
		zone = metadata.GetZone()
	}

	c, err := NewPowerVSCloudFunc(cloudInstanceId, zone, driverOptions.debug)
	if err != nil {
		klog.Fatalf("Failed to get powervs cloud: %v", err)
	}

	return controllerService{
		cloud:         c,
		driverOptions: driverOptions,
		volumeLocks:   util.NewVolumeLocks(),
	}
}

func (d *controllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume: called with args %+v", req)
	start := time.Now()
	volName := req.GetName()
	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume name not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volName); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volName)
	}
	defer d.volumeLocks.Release(volName)

	volSizeBytes, err := getVolSizeBytes(req)
	if err != nil {
		return nil, err
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	if !isValidVolumeCapabilities(volCaps) {
		modes := util.GetAccessModes(volCaps)
		stringModes := strings.Join(*modes, ", ")
		errString := fmt.Sprintf("Volume capabilities %s not supported. Only AccessModes [ReadWriteOnce], [ReadWriteMany], [ReadOnlyMany] supported.", stringModes)
		return nil, status.Error(codes.InvalidArgument, errString)
	}

	volumeType := cloud.DefaultVolumeType

	for key, value := range req.GetParameters() {
		switch strings.ToLower(key) {
		case VolumeTypeKey:
			volumeType = value
		default:
			return nil, status.Errorf(codes.InvalidArgument, "Invalid parameter key %s for CreateVolume", key)
		}
	}

	opts := &cloud.DiskOptions{
		Shareable:     isShareableVolume(volCaps),
		CapacityBytes: volSizeBytes,
		VolumeType:    volumeType,
	}

	if req.GetVolumeContentSource() != nil {
		return handleClone(d.cloud, req, volName, volSizeBytes, opts)
	}
	// Check if the disk already exists
	// Disk exists only if previous createVolume request fails due to any network/tcp error
	disk, err := d.cloud.GetDiskByName(volName)
	if disk != nil {
		// wait for volume to be available as the volume already exists
		klog.V(3).Infof("CreateVolume: Found an existing volume %s in %q state.", volName, disk.State)
		err := verifyVolumeDetails(opts, disk)
		if err != nil {
			return nil, err
		}
		if disk.State != cloud.VolumeAvailableState {
			vol, err := d.cloud.WaitForVolumeState(disk.VolumeID, cloud.VolumeAvailableState)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Disk exists, but not in required state. Current:%s Required:%s", disk.State, cloud.VolumeAvailableState)
			}
			// When the disk is still in the "Creating" state, the WWN will not be available.
			// In such a case, once when the volume is available, assign the WWN to the disk if not already assigned.
			if disk.WWN == "" {
				disk.WWN = vol.Wwn
			}
		}
	} else {
		if errors.Is(err, cloud.ErrNotFound) {
			disk, err = d.cloud.CreateDisk(volName, opts)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not create volume %q: %v", volName, err)
			}
		} else {
			return nil, status.Errorf(codes.Internal, "Could not find volume by name %q: %v", volName, err)
		}
	}
	klog.V(3).Infof("CreateVolume: created volume %s, took %s", volName, time.Since(start))
	return newCreateVolumeResponse(disk, req.VolumeContentSource), nil
}

func (d *controllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume: called with args: %+v", req)
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	if err := d.cloud.DeleteDisk(volumeID); err != nil {
		return nil, status.Errorf(codes.Internal, "Could not delete volume ID %q: %v", volumeID, err)
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (d *controllerService) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerPublishVolume: called with args %+v", req)
	start := time.Now()
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Node ID not provided")
	}
	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	caps := []*csi.VolumeCapability{volCap}
	if !isValidVolumeCapabilities(caps) {
		modes := util.GetAccessModes(caps)
		stringModes := strings.Join(*modes, ", ")
		errString := fmt.Sprintf("Volume capabilities %s not supported. Only AccessModes [ReadWriteOnce], [ReadWriteMany], [ReadOnlyMany] supported.", stringModes)
		return nil, status.Error(codes.InvalidArgument, errString)
	}

	pvInfo := map[string]string{WWNKey: req.VolumeContext[WWNKey]}

	err := d.cloud.AttachDisk(volumeID, nodeID)
	if err != nil {
		if strings.Contains(err.Error(), cloud.ErrConflictVolumeAlreadyExists.Error()) {
			return nil, status.Error(codes.AlreadyExists, err.Error())
		}
		if strings.Contains(err.Error(), cloud.ErrBadRequestVolumeNotFound.Error()) || strings.Contains(err.Error(), cloud.ErrPVInstanceNotFound.Error()) {
			return nil, status.Errorf(codes.NotFound, "Could not attach volume %q to node %q: %v", volumeID, nodeID, err)
		}
		return nil, status.Errorf(codes.Internal, "Could not attach volume %q to node %q: %v", volumeID, nodeID, err)
	}
	klog.V(4).Infof("ControllerPublishVolume: volume %s attached to node %s, took %s", volumeID, nodeID, time.Since(start))
	return &csi.ControllerPublishVolumeResponse{PublishContext: pvInfo}, nil
}

func (d *controllerService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerUnpublishVolume: called with args %+v", req)
	start := time.Now()
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	nodeID := req.GetNodeId()
	if nodeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Node ID not provided")
	}

	err := d.cloud.DetachDisk(volumeID, nodeID)
	if err != nil {
		if strings.Contains(err.Error(), cloud.ErrVolumeDetachNotFound.Error()) {
			klog.V(4).Infof("ControllerUnpublishVolume: volume %s is detached from node %s, took %s", volumeID, nodeID, time.Since(start))
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "Could not attach volume %q to node %q: %v", volumeID, nodeID, err)
	}
	// The volume in not associated, return success.
	klog.V(4).Infof("ControllerUnpublishVolume: volume %s is detached from node %s, took %s", volumeID, nodeID, time.Since(start))
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (d *controllerService) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(4).Infof("ControllerGetCapabilities: called with args %+v", req)
	caps := make([]*csi.ControllerServiceCapability, 0, len(controllerCaps))
	for _, cap := range controllerCaps {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *controllerService) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	klog.V(4).Infof("GetCapacity: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *controllerService) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.V(4).Infof("ListVolumes: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *controllerService) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.V(4).Infof("ValidateVolumeCapabilities: called with args %+v", req)
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volCaps := req.GetVolumeCapabilities()
	if len(volCaps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities not provided")
	}

	if _, err := d.cloud.GetDiskByID(volumeID); err != nil {
		if err == cloud.ErrNotFound {
			return nil, status.Error(codes.NotFound, "Volume not found")
		}
		return nil, status.Errorf(codes.Internal, "Could not get volume with ID %q: %v", volumeID, err)
	}

	var confirmed *csi.ValidateVolumeCapabilitiesResponse_Confirmed
	if isValidVolumeCapabilities(volCaps) {
		confirmed = &csi.ValidateVolumeCapabilitiesResponse_Confirmed{VolumeCapabilities: volCaps}
	}
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: confirmed,
	}, nil
}

func (d *controllerService) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	klog.V(4).Infof("ControllerExpandVolume: called with args %+v", req)
	volumeID := req.GetVolumeId()
	if volumeID == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	capRange := req.GetCapacityRange()
	if capRange == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range not provided")
	}

	newSize := util.RoundUpBytes(capRange.GetRequiredBytes())
	maxVolSize := capRange.GetLimitBytes()
	if maxVolSize > 0 && maxVolSize < newSize {
		return nil, status.Error(codes.InvalidArgument, "After round-up, volume size exceeds the limit specified")
	}

	actualSizeGiB, err := d.cloud.ResizeDisk(volumeID, newSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Could not resize volume %q: %v", volumeID, err)
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         util.GiBToBytes(actualSizeGiB),
		NodeExpansionRequired: true,
	}, nil
}

func (d *controllerService) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.V(4).Infof("ControllerGetVolume: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *controllerService) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	klog.V(4).InfoS("ControllerModifyVolume: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func isValidVolumeCapabilities(volCaps []*csi.VolumeCapability) bool {
	for _, c := range volCaps {
		mode := c.AccessMode.GetMode()
		if mode != SingleNodeWriter && mode != MultiNodeMultiWriter && mode != MultiNodeReaderOnly {
			return false
		}
	}
	return true
}

// Check if the volume is shareable.
func isShareableVolume(volCaps []*csi.VolumeCapability) bool {
	for _, c := range volCaps {
		mode := c.AccessMode.GetMode()
		if mode == MultiNodeMultiWriter || mode == MultiNodeReaderOnly {
			return true
		}
	}
	return false
}

func (d *controllerService) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	klog.V(4).Infof("CreateSnapshot: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *controllerService) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	klog.V(4).Infof("DeleteSnapshot: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func (d *controllerService) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	klog.V(4).Infof("ListSnapshots: called with args %+v", req)
	return nil, status.Error(codes.Unimplemented, "")
}

func newCreateVolumeResponse(disk *cloud.Disk, src *csi.VolumeContentSource) *csi.CreateVolumeResponse {
	volumeContext := map[string]string{
		DiskType:    disk.DiskType,
		WWNKey:      disk.WWN,
		DiskName:    disk.Name,
		IsShareable: strconv.FormatBool(disk.Shareable),
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      disk.VolumeID,
			CapacityBytes: util.GiBToBytes(disk.CapacityGiB),
			VolumeContext: volumeContext,
			ContentSource: src,
		},
	}
}

func getVolSizeBytes(req *csi.CreateVolumeRequest) (int64, error) {
	var volSizeBytes int64
	capRange := req.GetCapacityRange()
	if capRange == nil {
		volSizeBytes = cloud.DefaultVolumeSize
	} else {
		volSizeBytes = util.RoundUpBytes(capRange.GetRequiredBytes())
		maxVolSize := capRange.GetLimitBytes()
		if maxVolSize > 0 && maxVolSize < volSizeBytes {
			return 0, status.Error(codes.InvalidArgument, "After round-up, volume size exceeds the limit specified")
		}
	}
	return volSizeBytes, nil
}

func verifyVolumeDetails(payload *cloud.DiskOptions, diskDetails *cloud.Disk) error {
	if payload.Shareable != diskDetails.Shareable {
		return status.Errorf(codes.Internal, "Field mismatch for 'Shareable'. Payload:%+v, Available: %+v", payload.Shareable, diskDetails.Shareable)
	}
	if payload.VolumeType != diskDetails.DiskType {
		return status.Errorf(codes.Internal, "Field mismatch for 'VolumeType'. Payload:%+v, Available: %+v", payload.VolumeType, diskDetails.DiskType)
	}
	if util.BytesToGiB(payload.CapacityBytes) != diskDetails.CapacityGiB {
		return status.Errorf(codes.Internal, "Field mismatch for 'CapacityGiB'. Payload:%+v, Available: %+v", util.BytesToGiB(payload.CapacityBytes), diskDetails.CapacityGiB)
	}
	return nil
}

func handleClone(cloud cloud.Cloud, req *csi.CreateVolumeRequest, volName string, volSizeBytes int64, opts *cloud.DiskOptions) (*csi.CreateVolumeResponse, error) {
	volumeSource := req.VolumeContentSource
	switch volumeSource.Type.(type) {
	case *csi.VolumeContentSource_Volume:
		diskDetails, _ := cloud.GetDiskByNamePrefix("clone-" + req.GetName())
		if diskDetails != nil {
			if err := verifyVolumeDetails(opts, diskDetails); err != nil {
				return nil, err
			}
			return newCreateVolumeResponse(diskDetails, req.VolumeContentSource), nil
		}
		if srcVolume := volumeSource.GetVolume(); srcVolume != nil {
			var err error
			srcVolumeID := srcVolume.GetVolumeId()
			diskDetails, err = cloud.GetDiskByID(srcVolumeID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not get the source volume %q: %v", srcVolumeID, err)
			}
			if util.GiBToBytes(diskDetails.CapacityGiB) != volSizeBytes {
				return nil, status.Errorf(codes.Internal, "Cannot clone volume %v, source volume size is not equal to the clone volume", srcVolumeID)
			}
			if err = verifyVolumeDetails(opts, diskDetails); err != nil {
				return nil, err
			}
			diskFromSourceVolume, err := cloud.CloneDisk(srcVolumeID, volName)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not clone volume %q: %v", volName, err)
			}
			diskDetails, err = cloud.GetDiskByID(diskFromSourceVolume.VolumeID)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "Could not get volume %q after clone: %v", volName, err)
			}
		}
		return newCreateVolumeResponse(diskDetails, req.VolumeContentSource), nil
	}
	return nil, status.Errorf(codes.InvalidArgument, "%v not a proper volume source", volumeSource)
}
