package cloud

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	cloudInstanceIDLabel = "powervs.kubernetes.io/cloud-instance-id"
	pvmInstanceIdLabel   = "powervs.kubernetes.io/pvm-instance-id"
)

type KubernetesAPIClient func() (kubernetes.Interface, error)

// Get default kubernetes API client
var DefaultKubernetesAPIClient = func() (kubernetes.Interface, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

// Get testing kubernetes APIclient
var TestingKubernetesAPIClient = func() (kubernetes.Interface, error) {
	// Get configuration from config file
	config, err := clientcmd.BuildConfigFromFlags("", "/root/.kube/config")
	if err != nil {
		fmt.Println("ERROR building configuration:", err)
	}
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

// Get instance info from kubernetes API
func KubernetesAPIInstanceInfo(clientset kubernetes.Interface) (*Metadata, error) {
	nodeName := os.Getenv("CSI_NODE_NAME")
	if nodeName == "" {
		hostName, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		nodeName = hostName
	}
	if nodeName == "" {
		return nil, fmt.Errorf("CSI_NODE_NAME env var not set")
	}
	// get node with k8s API
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Node %s: %v", nodeName, err)
	}

	// Get node labels
	labels := node.GetLabels()
	keysList := []string{cloudInstanceIDLabel, pvmInstanceIdLabel}
	instanceInfo := Metadata{}
	for _, key := range keysList {
		if val, ok := labels[key]; ok {
			switch key {
			case cloudInstanceIDLabel:
				instanceInfo.cloudInstanceId = val
			case pvmInstanceIdLabel:
				instanceInfo.pvmInstanceId = val
			}
		} else {
			return nil, fmt.Errorf("error getting label %s for node Node %s", key, nodeName)
		}
	}

	return &instanceInfo, nil
}
