/*
Copyright 2021 The Kubernetes Authors.

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

package e2e

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
	powervscloud "sigs.k8s.io/ibm-powervs-block-csi-driver/pkg/cloud"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/tests/e2e/driver"
	"sigs.k8s.io/ibm-powervs-block-csi-driver/tests/e2e/testsuites"
)

const (
	defaultDiskSize   = 4
	defaultVolumeType = powervscloud.VolumeTypeTier3
	debug             = false
	apiKeyEnv         = "IBMCLOUD_API_KEY"
)

var (
	defaultDiskSizeBytes int64 = defaultDiskSize * 1024 * 1024 * 1024
)

var _ = Describe("[powervs-csi-e2e]Pre-Provisioned", func() {
	f := framework.NewDefaultFramework("powervs")

	var (
		cs                         clientset.Interface
		ns                         *v1.Namespace
		cloud                      powervscloud.Cloud
		volumeID                   string
		diskSize                   string
		pvTestDriver               driver.PVTestDriver
		skipManuallyDeletingVolume bool
	)

	BeforeEach(func() {
		cs = f.ClientSet
		ns = f.Namespace

		diskOptions := &powervscloud.DiskOptions{
			CapacityBytes: defaultDiskSizeBytes,
			VolumeType:    defaultVolumeType,
			Shareable:     false,
		}

		// setup Power VS volume
		if os.Getenv(apiKeyEnv) == "" {
			Skip(fmt.Sprintf("env %q not set", apiKeyEnv))
		}
		var err error
		cloudInstanceId, err := testsuites.GetCloudInstanceIdFromNodeLabels(cs)
		if err != nil {
			Skip(fmt.Sprintf("Could not get cloudInstanceId : %v", err))
		}

		cloud, err = powervscloud.NewPowerVSCloud(cloudInstanceId, debug)
		if err != nil {
			Fail(fmt.Sprintf("could not get NewCloud: %v", err))
		}

		r1 := rand.New(rand.NewSource(time.Now().UnixNano()))
		disk, err := cloud.CreateDisk(fmt.Sprintf("pvc-%d", r1.Uint64()), diskOptions)
		if err != nil {
			Fail(fmt.Sprintf("Create Disk failed: %v", err))
		}

		volumeID = disk.VolumeID
		diskSize = fmt.Sprintf("%dGi", defaultDiskSize)
		pvTestDriver = driver.InitPowervsCSIDriver()
	})

	AfterEach(func() {
		skipManuallyDeletingVolume = true
		if !skipManuallyDeletingVolume {
			err := cloud.WaitForVolumeState(volumeID, "detached")
			if err != nil {
				Fail(fmt.Sprintf("could not detach volume %q: %v", volumeID, err))
			}
			ok, err := cloud.DeleteDisk(volumeID)
			if err != nil || !ok {
				Fail(fmt.Sprintf("could not delete volume %q: %v", volumeID, err))
			}
		}
	})

	It("[env][labels] should write and read to a pre-provisioned volume", func() {
		pods := []testsuites.PodDetails{
			{
				Cmd: "echo 'hello world' > /mnt/test-1/data && grep 'hello world' /mnt/test-1/data",
				Volumes: []testsuites.VolumeDetails{
					{
						VolumeID:  volumeID,
						ClaimSize: diskSize,
						VolumeMount: testsuites.VolumeMountDetails{
							NameGenerate:      "test-volume-",
							MountPathGenerate: "/mnt/test-",
						},
					},
				},
			},
		}
		test := testsuites.PreProvisionedVolumeTest{
			CSIDriver: pvTestDriver,
			Pods:      pods,
		}
		test.Run(cs, ns)
	})
})
