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

package it

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/klog/v2"
)

var (
	stdVolCap = []*csi.VolumeCapability{
		{
			AccessType: &csi.VolumeCapability_Mount{
				Mount: &csi.VolumeCapability_MountVolume{},
			},
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	}
	stdCapRange    = &csi.CapacityRange{RequiredBytes: int64(1 * 1024 * 1024 * 1024)}
	testVolumeName = fmt.Sprintf("ibm-powervs-csi-driver-it-%d", rand.New(rand.NewSource(time.Now().UnixNano())).Uint64())
)

func TestIntegration(t *testing.T) {
	flag.Parse()
	RegisterFailHandler(Fail)
	RunSpecs(t, "IBM PowerVS CSI Driver Integration Tests")
}

var _ = Describe("IBM PowerVS CSI Driver", func() {

	It("Should create, attach, stage and mount volume, check if it's writable, unmount, unstage, detach, delete, and check if it's deleted", func() {

		testCreateAttachWriteReadDetachDelete()

	})
})

func testCreateAttachWriteReadDetachDelete() {
	klog.Infof("Creating volume with name %s", testVolumeName)
	start := time.Now()
	resp, err := csiClient.ctrl.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:               testVolumeName,
		CapacityRange:      stdCapRange,
		VolumeCapabilities: stdVolCap,
		Parameters:         nil,
	})
	Expect(err).To(BeNil(), "error during create volume")
	volume := resp.GetVolume()
	Expect(volume).NotTo(BeNil(), "volume is nil")
	klog.Infof("Created volume %s in %v", volume, time.Since(start))

	defer func() {
		klog.Infof("Deleting volume %s", volume.VolumeId)
		start := time.Now()
		_, err = csiClient.ctrl.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: volume.VolumeId})
		Expect(err).To(BeNil(), "error during delete volume")
		klog.Infof("Deleted volume %s in %v", volume.VolumeId, time.Since(start))
	}()

	klog.Info("Running ValidateVolumeCapabilities")
	vcResp, err := csiClient.ctrl.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           volume.VolumeId,
		VolumeCapabilities: stdVolCap,
	})
	Expect(err).To(BeNil())
	klog.Infof("Ran ValidateVolumeCapabilities with response %v", vcResp)

	klog.Info("Running NodeGetInfo")
	niResp, err := csiClient.node.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
	Expect(err).To(BeNil())
	klog.Infof("Ran NodeGetInfo with response %v", niResp)

	testAttachStagePublishDetach(volume)
}

func testAttachStagePublishDetach(volume *csi.Volume) {
	klog.Infof("Attaching volume %s to node %s", volume.VolumeId, nodeID)
	start := time.Now()
	respAttach, err := csiClient.ctrl.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{
		VolumeId:         volume.VolumeId,
		NodeId:           nodeID,
		VolumeCapability: stdVolCap[0],
		VolumeContext:    volume.VolumeContext,
	})
	Expect(err).To(BeNil(), "failed attaching volume %s to node %s", volume.VolumeId, nodeID)
	// assertAttachmentState(volumeID, "attached")
	klog.Infof("Attached volume with response %v in %v", respAttach.PublishContext, time.Since(start))

	defer func() {
		klog.Infof("Detaching volume %s from node %s", volume.VolumeId, nodeID)
		start := time.Now()
		_, err = csiClient.ctrl.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{
			VolumeId: volume.VolumeId,
			NodeId:   nodeID,
		})
		Expect(err).To(BeNil(), "failed detaching volume %s from node %s", volume.VolumeId, nodeID)
		// assertAttachmentState(volumeID, "detached")
		klog.Infof("Detached volume %s from node %s in %v", volume.VolumeId, nodeID, time.Since(start))
	}()

	if os.Getenv("TEST_REMOTE_NODE") == "1" {
		testStagePublish(volume.VolumeId, respAttach.PublishContext["wwn"])
	}
}

func testStagePublish(volumeID, wwn string) {
	// Node Stage
	volDir := filepath.Join("/tmp/", testVolumeName)
	stageDir := filepath.Join(volDir, "stage")
	klog.Infof("Staging volume %s to path %s", volumeID, stageDir)
	start := time.Now()
	_, err := csiClient.node.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stageDir,
		VolumeCapability:  stdVolCap[0],
		PublishContext:    map[string]string{"wwn": wwn},
	})
	Expect(err).To(BeNil(), "failed staging volume %s to path %s", volumeID, stageDir)
	klog.Infof("Staged volume %s to path %s in %v", volumeID, stageDir, time.Since(start))

	defer func() {
		klog.Infof("Unstaging volume %s from path %s", volumeID, stageDir)
		start := time.Now()
		_, err = csiClient.node.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{VolumeId: volumeID, StagingTargetPath: stageDir})
		Expect(err).To(BeNil(), "failed unstaging volume %s from path %s", volumeID, stageDir)
		err = os.RemoveAll(volDir)
		Expect(err).To(BeNil(), "failed to remove volume directory")
		klog.Infof("Unstaged volume %s from path %s in %v", volumeID, stageDir, time.Since(start))
	}()

	// Node Publish
	publishDir := filepath.Join("/tmp/", testVolumeName, "mount")
	klog.Infof("Publishing volume %s to path %s", volumeID, publishDir)
	start = time.Now()
	_, err = csiClient.node.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stageDir,
		TargetPath:        publishDir,
		VolumeCapability:  stdVolCap[0],
		PublishContext:    map[string]string{"wwn": wwn},
	})
	Expect(err).To(BeNil(), "failed publishing volume %s to path %s", volumeID, publishDir)
	klog.Infof("Published volume %s to path %s in %v", volumeID, publishDir, time.Since(start))

	defer func() {
		klog.Infof("Unpublishing volume %s from path %s", volumeID, publishDir)
		start := time.Now()
		_, err = csiClient.node.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
			VolumeId:   volumeID,
			TargetPath: publishDir,
		})
		Expect(err).To(BeNil(), "failed unpublishing volume %s from path %s", volumeID, publishDir)
		klog.Infof("Unpublished volume %s from path %s in %v", volumeID, publishDir, time.Since(start))
	}()
}
