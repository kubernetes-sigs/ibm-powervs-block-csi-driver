package testsuites

import (
	. "github.com/onsi/ginkgo"
	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"
	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// DynamicallyProvisionedDeletePodTest will provision required StorageClass and Deployment
// Testing if the Pod can write and read to mounted volumes
// Deleting a pod, and again testing if the Pod can write and read to mounted volumes
type DynamicallyProvisionedDeletePodTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Pod       PodDetails
	PodCheck  *PodExecCheck
}

type PodExecCheck struct {
	Cmd            []string
	ExpectedString string
}

func (t *DynamicallyProvisionedDeletePodTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	tDeployment, cleanup := t.Pod.SetupDeployment(client, namespace, t.CSIDriver)
	// defer must be called here for resources not get removed before using them
	for i := range cleanup {
		defer cleanup[i]()
	}

	By("deploying the deployment")
	tDeployment.Create()

	By("checking that the pod is running")
	tDeployment.WaitForPodReady()

	By("deleting the pod for deployment")
	tDeployment.DeletePodAndWait()

	By("checking again that the pod is running")
	tDeployment.WaitForPodReady()

	if t.PodCheck != nil {
		By("checking pod exec")
		tDeployment.Exec(t.PodCheck.Cmd, t.PodCheck.ExpectedString)
	}
}
