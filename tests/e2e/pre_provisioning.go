package e2e

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	. "github.com/onsi/ginkgo"
	powervscloud "github.com/ppc64le-cloud/powervs-csi-driver/pkg/cloud"
	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"
	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/testsuites"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	defaultDiskSize   = 4
	defaultVolumeType = powervscloud.VolumeTypeTier3
	dummyVolumeName   = "pre-provisioned"
	debug             = false
	apiKeyEnv         = "IBMCLOUD_API_KEY"
)

var (
	defaultDiskSizeBytes int64 = defaultDiskSize * 1024 * 1024 * 1024
)

var _ = Describe("[powervs-csi-e2e] [single-az] Pre-Provisioned", func() {
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
		metadata, err := powervscloud.NewMetadataService(powervscloud.TestingKubernetesAPIClient)
		if err != nil {
			Skip(fmt.Sprintf("Could not get Metadata : %v", err))
		}

		cloud, err = powervscloud.NewPowerVSCloud(metadata.GetServiceInstanceId(), debug)
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
			err := cloud.WaitForAttachmentState(volumeID, "detached")
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
