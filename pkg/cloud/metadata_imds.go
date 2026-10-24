/*
Copyright 2026 The Kubernetes Authors.

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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
)

const (
	// metadataBaseURL is the link-local endpoint for the PowerVS metadata service.
	// It is only reachable from within a PowerVS VSI that has the metadata service enabled.
	// Ref: https://www.ibm.com/docs/en/power-virtual-server?topic=deploying-configuring-managing-metadata-service-power-virtual-server
	metadataBaseURL = "https://api.metadata.power-iaas.cloud.ibm.com"
	tokenPath       = "/identity/v1/token"
	instancePath    = "/metadata/v1/instance"
	metadataFlavor  = "ibm"
)

// powerVSTokenResponse is the response body from PUT /identity/v1/token.
type powerVSTokenResponse struct {
	AccessToken string `json:"access_token"`
}

// powerVSInstanceResponse contains the fields we need from GET /metadata/v1/instance.
type powerVSInstanceResponse struct {
	PvmInstanceID     string `json:"pvmInstanceID"`
	CloudResourceName struct {
		ServiceInstance string `json:"serviceInstance"`
		Location        string `json:"location"`
	} `json:"cloudResourceName"`
}

// PowerVSMetadataService retrieves instance metadata from the PowerVS metadata service endpoint.
// It implements MetadataService.
type PowerVSMetadataService struct {
	meta powerVSInstanceResponse
}

var _ MetadataService = &PowerVSMetadataService{}

func (m *PowerVSMetadataService) GetZone() string { return m.meta.CloudResourceName.Location }
func (m *PowerVSMetadataService) GetCloudInstanceId() string {
	return m.meta.CloudResourceName.ServiceInstance
}
func (m *PowerVSMetadataService) GetPvmInstanceId() string { return m.meta.PvmInstanceID }

// GetRegion derives the region by stripping trailing digits and any trailing
// dash from the zone name.
// e.g. "osa21" = "osa", "us-south-1" = "us-south", "eu-de-1" = "eu-de".
func (m *PowerVSMetadataService) GetRegion() string {
	return strings.TrimRight(
		strings.TrimRightFunc(m.meta.CloudResourceName.Location, unicode.IsDigit), "-")
}

// doMetadataRequest executes req, checks that the response is 2xx,
// decodes the JSON body into out, and returns any error.
func doMetadataRequest(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// NewPowerVSMetadataService queries the PowerVS metadata service and returns
// a MetadataService populated with the current VSI's identity information.
// This only works when called from within a PowerVS VSI that has the metadata service enabled.
func NewPowerVSMetadataService() (MetadataService, error) {
	client := &http.Client{Timeout: 15 * time.Second}

	// Obtain an identity access token.
	tokenReq, err := http.NewRequestWithContext(context.Background(), http.MethodPut, metadataBaseURL+tokenPath, bytes.NewBufferString("{}"))
	if err != nil {
		return nil, fmt.Errorf("building PowerVS metadata token request: %w", err)
	}
	tokenReq.Header.Set("Content-Type", "application/json")
	tokenReq.Header.Set("Metadata-Flavor", metadataFlavor)

	var tokenBody powerVSTokenResponse
	if err := doMetadataRequest(client, tokenReq, &tokenBody); err != nil {
		return nil, fmt.Errorf("PowerVS metadata token request: %w", err)
	}

	// Retrieve instance metadata using the token.
	metaReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, metadataBaseURL+instancePath, nil)
	if err != nil {
		return nil, fmt.Errorf("building PowerVS metadata instance request: %w", err)
	}
	metaReq.Header.Set("Authorization", "Bearer "+tokenBody.AccessToken)
	metaReq.Header.Set("Metadata-Flavor", metadataFlavor)

	var instanceResp powerVSInstanceResponse
	if err := doMetadataRequest(client, metaReq, &instanceResp); err != nil {
		return nil, fmt.Errorf("PowerVS metadata instance request: %w", err)
	}

	if instanceResp.PvmInstanceID == "" {
		return nil, fmt.Errorf("PowerVS metadata response missing pvmInstanceID")
	}
	if instanceResp.CloudResourceName.ServiceInstance == "" {
		return nil, fmt.Errorf("PowerVS metadata response missing cloudResourceName.serviceInstance")
	}

	return &PowerVSMetadataService{meta: instanceResp}, nil
}
