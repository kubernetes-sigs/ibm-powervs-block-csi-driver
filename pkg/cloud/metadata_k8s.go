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

package cloud

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	CloudInstanceIDLabel = "powervs.kubernetes.io/cloud-instance-id"
	PvmInstanceIdLabel   = "powervs.kubernetes.io/pvm-instance-id"
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

// Get instance info from kubernetes API
func KubernetesAPIInstanceInfo(clientset kubernetes.Interface) (*Metadata, error) {
	nodeName := os.Getenv("CSI_NODE_NAME")
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
	keysList := []string{CloudInstanceIDLabel, PvmInstanceIdLabel}
	instanceInfo := Metadata{}
	for _, key := range keysList {
		if val, ok := labels[key]; ok {
			switch key {
			case CloudInstanceIDLabel:
				instanceInfo.cloudInstanceId = val
			case PvmInstanceIdLabel:
				instanceInfo.pvmInstanceId = val
			}
		} else {
			return nil, fmt.Errorf("error getting label %s for node Node %s", key, nodeName)
		}
	}

	return &instanceInfo, nil
}
