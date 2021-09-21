package cloud

import (
	"fmt"

	"k8s.io/klog/v2"
)

// Metadata is info about the instance on which the driver is running
type Metadata struct {
	cloudInstanceId string
	pvmInstanceId   string
}

// GetCloudInstanceId returns cloud instance id of the instance
func (m *Metadata) GetCloudInstanceId() string {
	return m.cloudInstanceId
}

// GetPvmInstanceId returns pvm instance id of the instance
func (m *Metadata) GetPvmInstanceId() string {
	return m.pvmInstanceId
}

// Get New Metadata Service
func NewMetadataService(k8sAPIClient KubernetesAPIClient) (MetadataService, error) {
	klog.Infof("retrieving instance data from kubernetes api")
	clientset, err := k8sAPIClient()
	if err != nil {
		klog.Warningf("error creating kubernetes api client: %v", err)
	} else {
		klog.Infof("kubernetes api is available")
		return KubernetesAPIInstanceInfo(clientset)
	}
	return nil, fmt.Errorf("error getting instance data from ec2 metadata or kubernetes api")
}
