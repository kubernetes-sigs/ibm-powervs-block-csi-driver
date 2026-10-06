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
	"encoding/json"
	"errors"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

// powerVSMetadataNewFunc is the constructor used to reach the PowerVS metadata service
// when a node's ProviderID is not set by the CCM.
var powerVSMetadataNewFunc = func() (MetadataService, error) {
	return NewPowerVSMetadataService()
}

type KubernetesAPIClient func(kubeconfig string) (kubernetes.Interface, error)

// Get default kubernetes API client.
var DefaultKubernetesAPIClient = func(kubeconfig string) (kubernetes.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, err
	}
	// creates the clientset
	return kubernetes.NewForConfig(config)
}

// Get instance info from kubernetes API.
func KubernetesAPIInstanceInfo(clientset kubernetes.Interface) (*Metadata, error) {
	nodeName := os.Getenv("CSI_NODE_NAME")
	if nodeName == "" {
		return nil, errors.New("CSI_NODE_NAME env var not set")
	}
	return GetInstanceInfoFromProviderID(clientset, nodeName)
}

func GetInstanceInfoFromProviderID(clientset kubernetes.Interface, nodeName string) (*Metadata, error) {
	// get node with k8s API
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("error getting Node %s: %v", nodeName, err)
	}

	if node.Spec.ProviderID != "" {
		providerId := node.Spec.ProviderID
		klog.Infof("Node Name: %s, Provider ID: %s", nodeName, providerId)
		return TokenizeProviderID(providerId)
	}

	klog.Warningf("ProviderID is empty for node %s, falling back to PowerVS metadata service", nodeName)
	svc, err := powerVSMetadataNewFunc()
	if err != nil {
		return nil, fmt.Errorf("ProviderID is empty for node %s and PowerVS metadata service fallback failed: %w", nodeName, err)
	}
	klog.Infof("PowerVS metadata service: region=%s zone=%s cloudInstanceID=%s pvmInstanceID=%s",
		svc.GetRegion(), svc.GetZone(), svc.GetCloudInstanceId(), svc.GetPvmInstanceId())

	// Patch the resolved ProviderID back onto the node so the controller and
	// subsequent startups can use it directly without calling the metadata service.
	// We only reach this path when node.Spec.ProviderID is already confirmed empty,
	// so a merge patch scoped to spec.providerID is safe and sufficient.
	providerID := fmt.Sprintf("ibmpowervs://%s/%s/%s/%s",
		svc.GetRegion(), svc.GetZone(), svc.GetCloudInstanceId(), svc.GetPvmInstanceId())
	type specPatch struct {
		Spec struct {
			ProviderID string `json:"providerID"`
		} `json:"spec"`
	}
	var sp specPatch
	sp.Spec.ProviderID = providerID
	patch, err := json.Marshal(sp)
	if err != nil {
		klog.Warningf("failed to marshal ProviderID patch for node %s: %v", nodeName, err)
	} else if _, patchErr := clientset.CoreV1().Nodes().Patch(
		context.TODO(), nodeName, types.MergePatchType, patch, metav1.PatchOptions{},
	); patchErr != nil {
		klog.Warningf("failed to patch ProviderID onto node %s: %v", nodeName, patchErr)
	} else {
		klog.Infof("patched ProviderID %s onto node %s", providerID, nodeName)
	}

	return &Metadata{
		region:          svc.GetRegion(),
		zone:            svc.GetZone(),
		cloudInstanceId: svc.GetCloudInstanceId(),
		pvmInstanceId:   svc.GetPvmInstanceId(),
	}, nil
}
