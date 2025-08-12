package cloud

import (
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/go-playground/assert/v2"
	"github.com/stretchr/testify/require"
)

func TestNewMetadataService(t *testing.T) {
	tests := []struct {
		name             string
		k8sAPIError      string
		providerID       string
		nodeName         string
		expectedError    string
		ProvideIDError   string
		expectedMetadata struct {
			pvmInstanceId   string
			region          string
			zone            string
			cloudInstanceId string
		}
	}{
		{
			name:       "Test NewMetadataService",
			nodeName:   "test-node",
			providerID: "ibmpowervs://region1/zone1/cloud-instance-123/instance-456",
			expectedMetadata: struct {
				pvmInstanceId   string
				region          string
				zone            string
				cloudInstanceId string
			}{
				pvmInstanceId:   "instance-456",
				region:          "region1",
				zone:            "zone1",
				cloudInstanceId: "cloud-instance-123",
			},
		},
		{
			name:          "k8s client error",
			k8sAPIError:   "failed to create client",
			expectedError: "failed to create client",
		},
		{
			name:          "missing CSI_NODE_NAME env var",
			providerID:    "ibmpowervs://region1/zone1/cloud-instance-123/instance-456",
			expectedError: "CSI_NODE_NAME env var not set",
		},
		{
			name:           "empty ProviderID",
			nodeName:       "test-node",
			providerID:     "",
			ProvideIDError: "ProviderID is empty",
			expectedError:  "ProviderID is empty",
		},
		{
			name:           "invalid providerID format",
			nodeName:       "test-node",
			providerID:     "ibmpowervs://region1/zone1/",
			ProvideIDError: "invalid ProviderID format",
			expectedError:  "invalid ProviderID format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set CSI_NODE_NAME if provided
			if tt.nodeName != "" {
				t.Setenv("CSI_NODE_NAME", tt.nodeName)
			}

			// Prepare fake k8s client factory
			mockK8sClient := func(_ string) (kubernetes.Interface, error) {
				if tt.k8sAPIError != "" {
					return nil, errors.New(tt.k8sAPIError)
				}
				if tt.nodeName == "" {
					return fake.NewSimpleClientset(), nil
				}
				node := &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.nodeName,
					},
					Spec: corev1.NodeSpec{
						ProviderID: tt.providerID,
					},
				}
				return fake.NewSimpleClientset(node), nil
			}

			metadata, err := NewMetadataService(mockK8sClient, "")

			if tt.expectedError != "" {
				require.ErrorContains(t, err, tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedMetadata.pvmInstanceId, metadata.GetPvmInstanceId())
			assert.Equal(t, tt.expectedMetadata.region, metadata.GetRegion())
			assert.Equal(t, tt.expectedMetadata.zone, metadata.GetZone())
			assert.Equal(t, tt.expectedMetadata.cloudInstanceId, metadata.GetCloudInstanceId())
		})
	}
}

func TestKubernetesAPIInstanceInfo(t *testing.T) {
	newNode := func(name, providerID string) *corev1.Node {
		return &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       corev1.NodeSpec{ProviderID: providerID},
		}
	}

	testCases := []struct {
		name             string
		nodeName         string
		node             *corev1.Node
		expectedError    string
		expectedMetadata *Metadata
	}{
		{
			name:          "TestKubernetesAPIInstanceInfo: Node name not set",
			nodeName:      "",
			expectedError: "CSI_NODE_NAME env var not set",
		},
		{
			name:          "Node not found",
			nodeName:      "missing-node",
			expectedError: "nodes \"missing-node\" not found",
		},
		{
			name:          "Node exists but ProviderID is empty",
			nodeName:      "test-node",
			node:          newNode("test-node", ""),
			expectedError: "ProviderID is empty",
		},
		{
			name:          "Invalid ProviderID Length",
			nodeName:      "test-node",
			node:          newNode("test-node", "ibmpowervs://too/short"),
			expectedError: "invalid length",
		},
		{
			name:          "TestKubernetesAPIInstanceInfo: Missing region",
			nodeName:      "test-node",
			node:          newNode("test-node", "ibmpowervs:///zone1/service_instance_id1/instance1"),
			expectedError: "region can't be empty",
		},
		{
			name:          "TestKubernetesAPIInstanceInfo: Missing Zone",
			nodeName:      "test-node",
			node:          newNode("test-node", "ibmpowervs://region1//service_instance_id1/instance1"),
			expectedError: "zone can't be empty",
		},
		{
			name:          "TestKubernetesAPIInstanceInfo: Missing service_instance_id",
			nodeName:      "test-node",
			node:          newNode("test-node", "ibmpowervs://region1/zone1//instance1"),
			expectedError: "service_instance_id can't be empty",
		},
		{
			name:          "TestKubernetesAPIInstanceInfo: Missing powervs_machine_id",
			nodeName:      "test-node",
			node:          newNode("test-node", "ibmpowervs://region1/zone1/cloud-instance1/"),
			expectedError: "powervs_machine_id can't be empty",
		},
		{
			name:     "TestKubernetesAPIInstanceInfo: Valid ProviderID",
			nodeName: "test-node",
			node:     newNode("test-node", "ibmpowervs://region1/zone1/service_instance_id1/instance1"),
			expectedMetadata: &Metadata{
				region:          "region1",
				zone:            "zone1",
				cloudInstanceId: "service_instance_id1",
				pvmInstanceId:   "instance1",
			},
		},
	}

	run := func(tc struct {
		name             string
		nodeName         string
		node             *corev1.Node
		expectedError    string
		expectedMetadata *Metadata
	}) {
		t.Setenv("CSI_NODE_NAME", tc.nodeName)

		clientset := fake.NewSimpleClientset()
		if tc.node != nil {
			clientset = fake.NewSimpleClientset(tc.node)
		}

		metadata, err := KubernetesAPIInstanceInfo(clientset)

		if tc.expectedError != "" {
			require.ErrorContains(t, err, tc.expectedError)
			require.Nil(t, metadata)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.expectedMetadata.region, metadata.region)
			require.Equal(t, tc.expectedMetadata.zone, metadata.zone)
			require.Equal(t, tc.expectedMetadata.cloudInstanceId, metadata.cloudInstanceId)
			require.Equal(t, tc.expectedMetadata.pvmInstanceId, metadata.pvmInstanceId)
		}
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) { run(tt) })
	}
}

func TestGetPvmInstanceId(t *testing.T) {
	metadata := &Metadata{
		pvmInstanceId: "pvminstance-123",
	}
	assert.Equal(t, "pvminstance-123", metadata.GetPvmInstanceId())
}

func TestGetCloudInstanceId(t *testing.T) {
	metadata := &Metadata{
		cloudInstanceId: "cloudinstance-123",
	}
	assert.Equal(t, "cloudinstance-123", metadata.GetCloudInstanceId())
}

func TestGetRegion(t *testing.T) {
	metadata := &Metadata{
		region: "us-south",
	}
	assert.Equal(t, "us-south", metadata.GetRegion())
}

func TestGetZone(t *testing.T) {
	metadata := &Metadata{
		zone: "us-south-1",
	}
	assert.Equal(t, "us-south-1", metadata.GetZone())
}
