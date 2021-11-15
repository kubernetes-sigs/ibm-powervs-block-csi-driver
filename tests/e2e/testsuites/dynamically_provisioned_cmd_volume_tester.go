package testsuites

import (
	. "github.com/onsi/ginkgo"
	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

type DynamicallyProvisionedCmdVolumeTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pods      []PodDetails
}

func (t *DynamicallyProvisionedCmdVolumeTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	for _, pod := range t.Pods {
		tpod, cleanup := pod.SetupWithDynamicVolumes(client, namespace, t.CSIDriver)
		// defer must be called here for resources not get removed before using them
		for i := range cleanup {
			defer cleanup[i]()
		}

		By("deploying the pod")
		tpod.Create()
		defer tpod.Cleanup()
		By("checking that the pods command exits with no error")
		tpod.WaitForSuccess()
	}
}
