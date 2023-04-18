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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/cloud"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/device"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/util"
)

const (

	// FSTypeExt2 represents the ext2 filesystem type
	FSTypeExt2 = "ext2"
	// FSTypeExt3 represents the ext3 filesystem type
	FSTypeExt3 = "ext3"
	// FSTypeExt4 represents the ext4 filesystem type
	FSTypeExt4 = "ext4"
	// FSTypeXfs represents te xfs filesystem type
	FSTypeXfs = "xfs"
	// default file system type to be used when it is not provided
	defaultFsType = "ext4"

	// defaultMaxVolumesPerInstance is the limit of volumes can be attached in the PowerVS environment
	// TODO: rightnow 99 is just a placeholder, this needs to be changed post discussion with PowerVS team
	defaultMaxVolumesPerInstance = 127 - 1

	// deviceInfoFileName is used to store the device details in a JSON file
	deviceInfoFileName = "deviceInfo.json"
)

var (
	// nodeCaps represents the capability of node service.
	nodeCaps = []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	}
)

// nodeService represents the node service of CSI driver
type nodeService struct {
	cloud         cloud.Cloud
	mounter       Mounter
	driverOptions *Options
	pvmInstanceId string
	volumeLocks   *util.VolumeLocks
	stats         StatsUtils
}

// newNodeService creates a new node service
// it panics if failed to create the service
func newNodeService(driverOptions *Options) nodeService {
	klog.V(4).Infof("retrieving node info from metadata service")
	metadata, err := cloud.NewMetadataService(cloud.DefaultKubernetesAPIClient, driverOptions.kubeconfig)
	if err != nil {
		panic(err)
	}

	pvsCloud, err := NewPowerVSCloudFunc(metadata.GetCloudInstanceId(), metadata.GetZone(), driverOptions.debug)
	if err != nil {
		panic(err)
	}

	return nodeService{
		cloud:         pvsCloud,
		mounter:       newNodeMounter(),
		driverOptions: driverOptions,
		pvmInstanceId: metadata.GetPvmInstanceId(),
		volumeLocks:   util.NewVolumeLocks(),
		stats:         &VolumeStatUtils{},
	}
}

func (d *nodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	klog.V(4).Infof("NodeStageVolume: called with args %+v", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetStagingTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	err := d.nodeStageVolume(req)
	if err != nil {
		return nil, err
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *nodeService) nodeStageVolume(req *csi.NodeStageVolumeRequest) error {

	target := req.GetStagingTargetPath()

	volCap := req.GetVolumeCapability()
	if !isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return status.Error(codes.InvalidArgument, "Volume capability not supported")
	}

	wwn, ok := req.PublishContext[WWNKey]
	if !ok || wwn == "" {
		return status.Error(codes.InvalidArgument, "WWN ID is not provided or empty")
	}

	// Stage the volume on the node by creating a new device with block or mount access.
	// If already staged, then validate it and return appropriate response.
	// Check if the volume has already been staged. If yes, then return here with success
	staged := d.isVolumeStaged(wwn, req)
	if staged {
		klog.V(4).Infof("volume has already been staged", "volumeID", req.VolumeId)
		return nil
	}

	// check if already mounted
	mounted, err := d.isDirMounted(target)
	if mounted {
		klog.V(4).Infof("mount already exists for staging target path %s", target, "volumeID", req.VolumeId)
		return nil
	}
	if err != nil {
		if os.IsNotExist(err) {
			klog.V(4).Infof("attempting mkdir for path %s", target, "volumeID", req.VolumeId)
			if err := os.MkdirAll(target, 0750); err != nil {
				return fmt.Errorf("mkdir failed for volumeID %s path %s (%v)", req.VolumeId, target, err)
			}
		} else {
			return err
		}
	}

	// Stage volume - Create device and expose volume as raw block or mounted directory (filesystem)
	klog.V(4).Infof("staging to the staging path %s", target, "volumeID", req.VolumeId)
	stagingDev, err := d.stageVolume(wwn, req)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("failed to stage volumeID %s err: %s", req.VolumeId, err))
	}
	if stagingDev == nil || stagingDev.Device == nil {
		return fmt.Errorf("invalid staging device info, staging device cannot be nil")
	}
	klog.V(4).Infof("staged successfully, StagingDev: %#v", stagingDev, "volumeID", req.VolumeId)

	// Save staged device info in the staging area
	klog.V(4).Infof("writing device info %+v to staging target path %s", stagingDev.Device, target, "volumeID", req.VolumeId)
	err = device.WriteData(target, deviceInfoFileName, stagingDev)
	if err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("Failed to stage volumeID %s, err: %v", req.VolumeId, err))
	}

	return nil
}

func (d *nodeService) isVolumeStaged(wwn string, req *csi.NodeStageVolumeRequest) bool {
	target := req.GetStagingTargetPath()

	// Read the device info from the staging path
	stagingDev, _ := device.ReadStagedDeviceInfo(target, deviceInfoFileName)
	if stagingDev == nil {
		return false
	}

	klog.V(4).Infof("found staged device: %+v", stagingDev, "volumeID", req.VolumeId)

	return req.VolumeId == stagingDev.VolumeID
}

func (d *nodeService) stageVolume(wwn string, req *csi.NodeStageVolumeRequest) (*device.StagingDevice, error) {
	klog.V(4).Infof(">>>>> stageVolume", "volumeID", req.VolumeId)
	defer klog.V(4).Infof("<<<<< stageVolume", "volumeID", req.VolumeId)

	target := req.GetStagingTargetPath()
	volCap := req.GetVolumeCapability()

	dev, err := d.setupDevice(wwn)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("error creating device for volumeID %s, err: %v", req.VolumeId, err))
	}

	klog.V(4).Infof("find device path", "volumeID", req.VolumeId, "device", dev.Mapper)

	// Construct staging device to be stored in the staging path on the node
	stagingDevice := &device.StagingDevice{
		VolumeID:         req.VolumeId,
		Device:           dev,
		VolumeAccessMode: "mount",
	}

	// If the access type is block, do nothing for stage
	switch volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		stagingDevice.VolumeAccessMode = "block"
		return stagingDevice, nil
	}

	// collect mount options
	mnt := volCap.GetMount()
	if mnt == nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("mnt is nil within volume capability for volumeID %s", req.VolumeId))
	}
	fsType := mnt.GetFsType()
	if len(fsType) == 0 {
		fsType = defaultFsType
	}
	var mountOptions []string
	for _, f := range mnt.MountFlags {
		if !hasMountOption(mountOptions, f) {
			mountOptions = append(mountOptions, f)
		}
	}

	// Check if a device is mounted in target directory
	deviceFromMount, _, err := mount.GetDeviceNameFromMount(d.mounter, target)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to check if device is already mounted for volumeID %s: %v", req.VolumeId, err))
	}

	// This operation (NodeStageVolume) MUST be idempotent.
	// If the volume corresponding to the volume_id is already staged to the staging_target_path,
	// and is identical to the specified volume_capability the Plugin MUST reply 0 OK.
	source := dev.Mapper
	if err == nil && deviceFromMount == source {
		klog.V(4).Infof("Volume is already staged", "volumeID", req.VolumeId)
		return stagingDevice, nil
	}

	// FormatAndMount will format only if needed
	klog.V(5).Infof("starting formatting %s and mounting at %s with fstype %s", source, target, fsType, "volumeID", req.VolumeId)
	err = d.mounter.FormatAndMount(source, target, fsType, mountOptions)
	if err != nil {
		msg := fmt.Sprintf("could not format %q and mnt it at %q for volumeID %s with err %v", source, target, req.VolumeId, err)
		return nil, status.Error(codes.Internal, msg)
	}
	klog.V(5).Infof("completed formatting %s and mounting at %s with fstype %s for wwn %s", source, target, fsType, "volumeID", req.VolumeId)

	// populate mount info in the staging device
	mountInfo := &device.Mount{
		Mountpoint: target,
		Options:    mountOptions,
		Device:     dev,
		FSType:     fsType,
	}
	stagingDevice.MountInfo = mountInfo

	return stagingDevice, nil
}

func (d *nodeService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(4).Infof("NodeUnstageVolume: called with args %+v", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetStagingTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	// Unstage the volume from the staging area
	if err := d.nodeUnstageVolume(req); err != nil {
		klog.Errorf("Failed to unstage volume %s, err: %v", volumeID, err)
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}
func (d *nodeService) nodeUnstageVolume(req *csi.NodeUnstageVolumeRequest) error {
	klog.V(5).Infof(">>>>> nodeUnstageVolume", "volumeID", req.VolumeId)
	defer klog.Info("<<<<< nodeUnstageVolume", "volumeID", req.VolumeId)

	volumeID := req.VolumeId
	target := req.GetStagingTargetPath()

	// Check if the staged device file exists and read
	deviceFilePath := path.Join(target, deviceInfoFileName)
	exists, _, _ := device.FileExists(deviceFilePath)
	if !exists {
		klog.V(5).Infof("volume %s not in staged state as the device info file %s does not exist", volumeID, deviceFilePath)
		return nil
	}
	stagingDev, _ := device.ReadStagedDeviceInfo(target, deviceInfoFileName)
	if stagingDev == nil {
		klog.Infof("volume %s not in staged state as the staging device info file %s does not exist", volumeID, deviceFilePath)
		return nil
	}

	klog.Infof("found staged device info: %+v", stagingDev, "volumeID", req.VolumeId)

	dev := stagingDev.Device
	if dev == nil {
		return status.Error(codes.Internal, fmt.Sprintf("missing device info in the staging device %v for volumeID %s", stagingDev, volumeID))
	}

	// If mounted, then unmount the filesystem
	if stagingDev.VolumeAccessMode == "mount" && stagingDev.MountInfo != nil {
		klog.V(5).Infof("starting unmounting %s", target, "volumeID", volumeID)
		err := mount.CleanupMountPoint(target, d.mounter, true)
		if err != nil {
			return status.Errorf(codes.Internal, "failed to unmount for vol %s target %q: %v", volumeID, target, err)
		}
		klog.V(5).Infof("completed unmounting %s", target, "volumeID", volumeID)
	}

	// Delete device
	klog.Infof("deleting device %+v", dev, "volumeID", volumeID)
	//check if device is mounted or has holders
	err := d.checkIfDeviceCanBeDeleted(dev)
	if err != nil {
		return fmt.Errorf("failed to delete device for volumeID %s: %v", volumeID, err)
	}

	if err := device.DeleteDevice(dev); err != nil {
		return status.Error(codes.Internal, fmt.Sprintf("error deleting device %s for volumeID %s: %v", dev.Mapper, volumeID, err))
	}
	// Remove the device file
	device.FileDelete(deviceFilePath)

	return nil
}

func (d *nodeService) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	klog.V(4).Infof("NodeExpandVolume: called with args %+v", *req)

	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volumePath := req.GetVolumePath()
	if len(volumePath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume path not provided")
	}

	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, volumeID)
	}
	defer d.volumeLocks.Release(volumeID)

	volumeCapability := req.GetVolumeCapability()
	isBlock := false

	// VolumeCapability is optional, if specified, use that as source of truth.
	if volumeCapability != nil {
		if !isValidVolumeCapabilities([]*csi.VolumeCapability{volumeCapability}) {
			return nil, status.Error(codes.InvalidArgument, "Volume capability not supported")
		}
		isBlock = volumeCapability.GetBlock() != nil
	} else {
		// VolumeCapability is nil, check if volumePath points to a block device.
		var err error
		isBlock, err = d.stats.IsBlockDevice(volumePath)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to determine if volume path %v is a block device: %v", volumePath, err, "volumeID", volumeID)
		}
	}

	// Noop for block NodeExpandVolume.
	if isBlock {
		klog.V(4).Infof("NodeExpandVolume: ignoring as given volume path is a block device", "volumeID", volumeID, "volumePath", volumePath)
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	notMounted, err := d.mounter.IsLikelyNotMountPoint(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "volume path %s check for mount failed: %v", volumePath, volumeID, err)
	}

	if notMounted {
		return nil, status.Errorf(codes.Internal, "volume path %s is not mounted", volumePath, "volumeID", volumeID)
	}

	devicePath, _, err := mount.GetDeviceNameFromMount(d.mounter, volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get device from volume path %s: %v", volumePath, err, "volumeID", volumeID)
	}

	if devicePath == "" {
		return nil, status.Errorf(codes.Internal, "failed to get device from volume path %s", volumePath, "volumeID", volumeID)
	}

	// TODO: refactor Mounter to expose a mount.SafeFormatAndMount object
	r := mount.NewResizeFs(d.mounter.(*NodeMounter).Exec)

	// TODO: lock per volume ID to have some idempotency
	if _, err := r.Resize(devicePath, volumePath); err != nil {
		return nil, status.Errorf(codes.Internal, "could not resize volume %q (%q):  %v", volumeID, devicePath, err)
	}

	return &csi.NodeExpandVolumeResponse{}, nil
}

func (d *nodeService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).Infof("NodePublishVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	source := req.GetStagingTargetPath()
	if len(source) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Staging target not provided")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	// Acquire a lock on the target path instead of volumeID, since we do not want to serialize multiple node publish calls on the same volume.
	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, target)
	}
	defer d.volumeLocks.Release(volumeID)

	volCap := req.GetVolumeCapability()
	if volCap == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not provided")
	}

	if !isValidVolumeCapabilities([]*csi.VolumeCapability{volCap}) {
		return nil, status.Error(codes.InvalidArgument, "Volume capability not supported")
	}

	mountOptions := []string{"bind"}
	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	switch mode := volCap.GetAccessType().(type) {
	case *csi.VolumeCapability_Block:
		if err := d.nodePublishVolumeForBlock(req, mountOptions); err != nil {
			return nil, err
		}
	case *csi.VolumeCapability_Mount:
		if err := d.nodePublishVolumeForFileSystem(req, mountOptions, mode); err != nil {
			return nil, err
		}
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *nodeService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume: called with args %+v", *req)
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	target := req.GetTargetPath()
	if len(target) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path not provided")
	}

	// Acquire a lock on the target path instead of volumeID, since we do not want to serialize multiple node publish calls on the same volume.
	if acquired := d.volumeLocks.TryAcquire(volumeID); !acquired {
		return nil, status.Errorf(codes.Aborted, util.VolumeOperationAlreadyExistsFmt, target)
	}
	defer d.volumeLocks.Release(volumeID)

	klog.V(5).Infof("starting unmounting %s for volumeID %s", target, volumeID)
	err := mount.CleanupMountPoint(target, d.mounter, false)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not unmount %q for volumeID %s volumeID: %v", target, volumeID, err)
	}
	klog.V(5).Infof("completed unmounting %s for volumeID %s", target, volumeID)

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *nodeService) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	var resp *csi.NodeGetVolumeStatsResponse
	if req != nil {
		klog.V(4).Infof("NodeGetVolumeStats: called with args %+v", *req)
	}

	if req == nil || req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	if req.VolumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumePath not provided")
	}

	volumePath := req.VolumePath
	// return if path does not exist
	if d.stats.IsPathNotExist(volumePath) {
		return nil, status.Error(codes.NotFound, "VolumePath not exist")
	}

	// check if volume mode is raw volume mode
	isBlock, err := d.stats.IsBlockDevice(volumePath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to check volume %s is block device or not: %v", req.VolumeId, err))
	}
	// if block device, get deviceStats
	if isBlock {
		capacity, err := d.stats.DeviceInfo(volumePath)
		if err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to collect block device info: %v", err))
		}

		resp = &csi.NodeGetVolumeStatsResponse{
			Usage: []*csi.VolumeUsage{
				{
					Total: capacity,
					Unit:  csi.VolumeUsage_BYTES,
				},
			},
		}

		klog.V(4).Infof("Block Device Volume stats collected: %+v\n", resp)
		return resp, nil
	}

	// else get the file system stats
	available, capacity, usage, inodes, inodesFree, inodesUsed, err := d.stats.FSInfo(volumePath)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to collect FSInfo: %v", err))
	}
	resp = &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: available,
				Total:     capacity,
				Used:      usage,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: inodesFree,
				Total:     inodes,
				Used:      inodesUsed,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}

	klog.V(4).Infof("FS Volume stats collected: %+v\n", resp)
	return resp, nil
}

func (d *nodeService) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(4).Infof("NodeGetCapabilities: called with args %+v", *req)
	var caps []*csi.NodeServiceCapability
	for _, cap := range nodeCaps {
		c := &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: cap,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *nodeService) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).Infof("NodeGetInfo: called with args %+v", *req)

	in, err := d.cloud.GetPVMInstanceByID(d.pvmInstanceId)
	if err != nil {
		klog.Errorf("failed to get the instance for pvmInstanceId %s, err: %s", d.pvmInstanceId, err)
		return nil, fmt.Errorf("failed to get the instance for pvmInstanceId %s, err: %s", d.pvmInstanceId, err)
	}
	image, err := d.cloud.GetImageByID(in.ImageID)
	if err != nil {
		return nil, fmt.Errorf("failed to get the image details for %s, err: %s", in.ImageID, err)
	}

	segments := map[string]string{
		DiskTypeKey: image.DiskType,
	}

	topology := &csi.Topology{Segments: segments}

	return &csi.NodeGetInfoResponse{
		NodeId:             d.pvmInstanceId,
		MaxVolumesPerNode:  d.getVolumesLimit(),
		AccessibleTopology: topology,
	}, nil
}

func (d *nodeService) nodePublishVolumeForBlock(req *csi.NodePublishVolumeRequest, mountOptions []string) error {
	klog.V(5).Infof(">>>>> nodePublishVolumeForBlock", "volumeID", req.VolumeId)
	defer klog.Info("<<<<< nodePublishVolumeForBlock", "volumeID", req.VolumeId)

	target := req.GetTargetPath()
	volumeID := req.GetVolumeId()

	// Read device info from the staging area
	stagingDev, err := device.ReadStagedDeviceInfo(req.GetStagingTargetPath(), deviceInfoFileName)
	if err != nil {
		return status.Error(codes.FailedPrecondition,
			fmt.Sprintf("staging target path %s not set for volumeID %s, err: %v", target, req.VolumeId, err))
	}
	if stagingDev == nil || stagingDev.Device == nil {
		return status.Error(codes.FailedPrecondition,
			fmt.Sprintf("staging device is not configured at the staging path %s for volumeID %s", target, volumeID))
	}

	source := stagingDev.Device.Mapper
	klog.V(4).Infof("[block]: found device path for volumeID %s -> %s", volumeID, source)

	// create the global mount path if it is missing
	// Path in the form of /var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/{volumeName}
	globalMountPath := filepath.Dir(target)
	exists, err := d.mounter.ExistsPath(globalMountPath)
	if err != nil {
		return status.Errorf(codes.Internal, "could not check if path exists %q for volumeID %s: %v", globalMountPath, volumeID, err)
	}

	if !exists {
		if err = d.mounter.MakeDir(globalMountPath); err != nil {
			return status.Errorf(codes.Internal, "could not create dir %q for volumeID %s: %v", globalMountPath, volumeID, err)
		}
	}

	// Create the mount point as a file since bind mount device node requires it to be a file
	klog.V(5).Infof("[block]: making target file %s for volumeID %s", target, volumeID)
	err = d.mounter.MakeFile(target)
	if err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "[block]: could not remove mount target %q for volumeID %s: %v", target, volumeID, removeErr)
		}
		return status.Errorf(codes.Internal, "[block]: could not create file %q for volumeID %s: %v", target, volumeID, err)
	}

	klog.V(5).Infof("[block]: starting mounting %s at %s for volumeID %s", source, target, volumeID)
	if err := d.mounter.Mount(source, target, "", mountOptions); err != nil {
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "[block]: could not remove mount target %q for volumeID %s: %v", target, volumeID, removeErr)
		}
		return status.Errorf(codes.Internal, "[block]: could not mount %q at %q for volumeID %s: %v", source, target, volumeID, err)
	}
	klog.V(5).Infof("[block]: completed mounting %s at %s for volumeID %s", source, target, volumeID)

	return nil
}

func (d *nodeService) nodePublishVolumeForFileSystem(req *csi.NodePublishVolumeRequest, mountOptions []string, mode *csi.VolumeCapability_Mount) error {
	klog.V(5).Infof(">>>>> nodePublishVolumeForFileSystem", "volumeID", req.VolumeId)
	defer klog.Info("<<<<< nodePublishVolumeForFileSystem", "volumeID", req.VolumeId)

	target := req.GetTargetPath()
	source := req.GetStagingTargetPath()
	volumeID := req.VolumeId

	if err := d.mounter.MakeDir(target); err != nil {
		return status.Errorf(codes.Internal, "could not create dir %q for volumeID %s: %v", target, volumeID, err)
	}

	fsType := mode.Mount.GetFsType()
	if len(fsType) == 0 {
		fsType = defaultFsType
	}

	klog.V(5).Infof("starting mounting %s at %s with option %s as fstype %s for volumeID %s", source, target, mountOptions, fsType, volumeID)
	if err := d.mounter.Mount(source, target, fsType, mountOptions); err != nil {
		notMnt, mntErr := d.mounter.IsLikelyNotMountPoint(target)
		if mntErr != nil {
			return status.Errorf(codes.Internal, "error when validating mount path %s for volumeID %s: %v", target, volumeID, mntErr)
		}
		if !notMnt {
			if mntErr = d.mounter.Unmount(target); mntErr != nil {
				return status.Errorf(codes.Internal, "failed to unmount path %s for volumeID %s: %v", target, volumeID, mntErr)
			}
			notMnt, mntErr = d.mounter.IsLikelyNotMountPoint(target)
			if mntErr != nil {
				return status.Errorf(codes.Internal, "error when validating mount path %s for volumeID %s: %v", target, volumeID, mntErr)
			}
			if !notMnt {
				// This is very odd, we don't expect it.  We'll try again next sync loop.
				return status.Errorf(codes.Internal, "failed to unmount path %s for volumeID %s: %v", target, volumeID, err)
			}
		}
		if removeErr := os.Remove(target); removeErr != nil {
			return status.Errorf(codes.Internal, "could not remove mount target %q for volumeID %s: %v", target, volumeID, err)
		}
		return status.Errorf(codes.Internal, "could not mount %q at %q for volumeID %s: %v", source, target, volumeID, err)
	}
	klog.V(5).Infof("completed mounting %s at %s with option %s as fstype %s for volumeID %s", source, target, mountOptions, fsType, volumeID)

	return nil
}

// getVolumesLimit returns the limit of volumes that the node supports
func (d *nodeService) getVolumesLimit() int64 {
	if d.driverOptions.volumeAttachLimit >= 0 {
		return d.driverOptions.volumeAttachLimit
	}
	return defaultMaxVolumesPerInstance
}

// hasMountOption returns a boolean indicating whether the given
// slice already contains a mount option. This is used to prevent
// passing duplicate option to the mount command.
func hasMountOption(options []string, opt string) bool {
	for _, o := range options {
		if o == opt {
			return true
		}
	}
	return false
}

// isDirMounted checks if the path is already a mount point
func (d *nodeService) isDirMounted(target string) (bool, error) {
	// Check if mount already exists
	// TODO(msau): check why in-tree uses IsNotMountPoint
	// something related to squash and not having permissions to lstat
	notMnt, err := d.mounter.IsLikelyNotMountPoint(target)
	if err != nil {
		return false, err
	}
	if !notMnt {
		// Already mounted
		return true, nil
	}
	return false, nil
}

func (d *nodeService) setupDevice(wwn string) (*device.Device, error) {
	dev := device.GetDeviceFromVolume(wwn)
	if dev != nil {
		err := device.DeleteDevice(dev)
		if err != nil {
			klog.Warningf("failed to cleanup stale device %s before staging for WWN %s, err %v", dev.Mapper, dev.WWN, err)
		}
	}

	// Create Device
	dev = nil
	dev, err := device.CreateDevice(wwn)
	if err != nil {
		return nil, fmt.Errorf("error creating device for wwn %s, err: %v", wwn, err)
	}
	if dev == nil {
		return nil, fmt.Errorf("unable to find the device for wwn %s", wwn)
	}

	return dev, err
}

// checkIfDeviceCanBeDeleted: check if device is currently in use
func (d *nodeService) checkIfDeviceCanBeDeleted(dev *device.Device) (err error) {
	// first check if the device is already mounted
	ml, err := d.mounter.List()
	if err != nil {
		return err
	}

	for _, mount := range ml {
		if strings.EqualFold(dev.Mapper, mount.Device) || strings.EqualFold(dev.Pathname, mount.Device) {
			return fmt.Errorf("%s is currently mounted for wwn %s", dev.Mapper, dev.WWN)
		}
	}

	return nil
}
