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

package driver

import (
	"fmt"
	"reflect"
	"testing"
)

func TestValidateMode(t *testing.T) {
	testCases := []struct {
		name   string
		mode   Mode
		expErr error
	}{
		{
			name:   "valid mode: all",
			mode:   AllMode,
			expErr: nil,
		},
		{
			name:   "valid mode: controller",
			mode:   ControllerMode,
			expErr: nil,
		},
		{
			name:   "valid mode: node",
			mode:   NodeMode,
			expErr: nil,
		},
		{
			name:   "invalid mode: unknown",
			mode:   Mode("unknown"),
			expErr: fmt.Errorf("Mode is not supported (actual: unknown, supported: %v)", []Mode{AllMode, ControllerMode, NodeMode}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMode(tc.mode)
			if !reflect.DeepEqual(err, tc.expErr) {
				t.Fatalf("error not equal\ngot:\n%s\nexpected:\n%s", err, tc.expErr)
			}
		})
	}
}

func TestValidateDriverOptions(t *testing.T) {
	testCases := []struct {
		name            string
		mode            Mode
		extraVolumeTags map[string]string
		expErr          error
	}{
		{
			name:   "success",
			mode:   AllMode,
			expErr: nil,
		},
		{
			name:   "fail because validateMode fails",
			mode:   Mode("unknown"),
			expErr: fmt.Errorf("invalid mode: Mode is not supported (actual: unknown, supported: %v)", []Mode{AllMode, ControllerMode, NodeMode}),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateDriverOptions(&Options{
				extraTags: tc.extraVolumeTags,
				mode:      tc.mode,
			})
			if !reflect.DeepEqual(err, tc.expErr) {
				t.Fatalf("error not equal\ngot:\n%s\nexpected:\n%s", err, tc.expErr)
			}
		})
	}
}
