package testsuites

import (
	"github.com/ppc64le-cloud/powervs-csi-driver/pkg/cloud"
	"github.com/ppc64le-cloud/powervs-csi-driver/tests/e2e/driver"

	v1 "k8s.io/api/core/v1"
	clientset "k8s.io/client-go/kubernetes"
)

// DynamicallyProvisionedReclaimPolicyTest will provision required PV(s) and PVC(s)
// Testing the correct behavior for different reclaimPolicies
type DynamicallyProvisionedReclaimPolicyTest struct {
	CSIDriver driver.DynamicPVTestDriver
	Volumes   []VolumeDetails
	Cloud     cloud.Cloud
}

func (t *DynamicallyProvisionedReclaimPolicyTest) Run(client clientset.Interface, namespace *v1.Namespace) {
	for _, volume := range t.Volumes {
		tpvc, _ := volume.SetupDynamicPersistentVolumeClaim(client, namespace, t.CSIDriver)

		// will delete the PVC
		// will also wait for PV to be deleted separately when reclaimPolicy=Retian
		tpvc.Cleanup()
		// first check PV stills exists, then manually delete it
		if tpvc.ReclaimPolicy() == v1.PersistentVolumeReclaimRetain {
			tpvc.WaitForPersistentVolumePhase(v1.VolumeReleased)
			tpvc.DeleteBoundPersistentVolume()
			tpvc.DeleteBackingVolume(t.Cloud)
		}
	}
}
