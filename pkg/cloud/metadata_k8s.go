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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
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
	return GetInstanceInfoFromProviderID(clientset, nodeName)
}

func GetInstanceInfoFromProviderID(clientset kubernetes.Interface, nodeName string) (*Metadata, error) {
	// get node with k8s API
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Node %s: %v", nodeName, err)
	}

	instanceInfo := Metadata{}
	// ProviderID format: ibmpowervs://<region>/<zone>/<service_instance_id>/<powervs_machine_id>
	if node.Spec.ProviderID != "" {
		providerId := node.Spec.ProviderID
		klog.Infof("Node Name: %s, Provider ID: %s", nodeName, providerId)
		data := strings.Split(providerId, "/")
		if len(data) != ProviderIDValidLength {
			return nil, fmt.Errorf("invalid ProviderID format - %v, expected format - ibmpowervs://<region>/<zone>/<service_instance_id>/<powervs_machine_id>", providerId)
		}
		instanceInfo.cloudInstanceId = data[4]
		instanceInfo.pvmInstanceId = data[5]
	} else {
		return nil, fmt.Errorf("ProviderID is empty for the node: %s", nodeName)
	}

	return &instanceInfo, nil
}
