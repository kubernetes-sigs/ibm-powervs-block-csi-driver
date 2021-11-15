package testsuites

import (
	"fmt"

	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"

	. "github.com/onsi/ginkgo"
)

// DynamicallyProvisionedTopologyAwareVolumeTest will provision required StorageClass(es), PVC(s) and Pod(s)
// Waiting for the PV provisioner to create a new PV
// Testing if the Pod(s) can write and read to mounted volumes
// Validate PVs have expected PV nodeAffinity
type DynamicallyProvisionedTopologyAwareVolumeTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pods      []PodDetails
}

func (t *DynamicallyProvisionedTopologyAwareVolumeTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	for _, pod := range t.Pods {
		tpod := NewTestPod(client, namespace, pod.Cmd)
		fmt.Printf("testpod : %+v", tpod)

		tpvcs := make([]*TestPersistentVolumeClaim, len(pod.Volumes))
		for n, v := range pod.Volumes {
			var cleanup []func()
			tpvcs[n], cleanup = v.SetupDynamicPersistentVolumeClaim(client, namespace, t.CSIDriver)
			for i := range cleanup {
				defer cleanup[i]()
			}
			tpod.SetupVolume(tpvcs[n].persistentVolumeClaim, fmt.Sprintf("%s%d", v.VolumeMount.NameGenerate, n+1), fmt.Sprintf("%s%d", v.VolumeMount.MountPathGenerate, n+1), v.VolumeMount.ReadOnly)
		}

		By("deploying the pod")
		tpod.Create()
		defer tpod.Cleanup()

		By("checking that the pods command exits with no error")
		tpod.WaitForSuccess()
		By("validating provisioned PVs")
		for n := range tpvcs {
			tpvcs[n].WaitForBound()
			tpvcs[n].ValidateProvisionedPersistentVolume()
		}
		fmt.Println("Cleanup now")

	}
}
