package testsuites

import (
	. "github.com/onsi/ginkgo"
	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// PreProvisionedVolumeTest will provision required PV(s), PVC(s) and Pod(s)
// Testing if the Pod(s) can write and read to mounted volumes
type PreProvisionedVolumeTest struct {
	CSIDriver driver.PreProvisionedVolumeTestDriver
	Pods      []PodDetails
}

func (t *PreProvisionedVolumeTest) Run(client clientset.Interface, namespace *v1.Namespace) {

	for _, pod := range t.Pods {
		tpod, cleanup := pod.SetupWithPreProvisionedVolumes(client, namespace, t.CSIDriver)
		// defer must be called here for resources not get removed before using them
		for i := range cleanup {
			defer cleanup[i]()
		}
		By("deploying the pod")
		tpod.Create()

		By("checking that the pods command exits with no error")
		tpod.WaitForSuccess()
		defer tpod.Cleanup()
	}
}
