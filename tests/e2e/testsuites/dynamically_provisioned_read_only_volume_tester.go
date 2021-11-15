package testsuites

import (
	"fmt"

	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	"k8s.io/kubernetes/test/e2e/framework"

	clientset "k8s.io/client-go/kubernetes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

const expectedReadOnlyLog = "Read-only file system"

// DynamicallyProvisionedReadOnlyVolumeTest will provision required StorageClass(es), PVC(s) and Pod(s)
// Waiting for the PV provisioner to create a new PV
// Testing that the Pod(s) cannot write to the volume when mounted
type DynamicallyProvisionedReadOnlyVolumeTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pods      []PodDetails
}

func (t *DynamicallyProvisionedReadOnlyVolumeTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	for _, pod := range t.Pods {
		tpod, cleanup := pod.SetupWithDynamicVolumes(client, namespace, t.CSIDriver)
		// defer must be called here for resources not get removed before using them
		for i := range cleanup {
			defer cleanup[i]()
		}

		By("deploying the pod")
		tpod.Create()
		defer tpod.Cleanup()
		By("checking that the pods command exits with an error")
		tpod.WaitForFailure()
		By("checking that pod logs contain expected message")
		body, err := tpod.Logs()
		framework.ExpectNoError(err, fmt.Sprintf("Error getting logs for pod %s: %v", tpod.pod.Name, err))
		Expect(string(body)).To(ContainSubstring(expectedReadOnlyLog))
	}
}
