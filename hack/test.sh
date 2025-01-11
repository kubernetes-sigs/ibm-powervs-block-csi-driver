#!/bin/bash

# Copyright 2022 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o pipefail

function produce_junit_xmlreport {
  if [[ -z "${ARTIFACTS}" ]]; then
    return
  fi

  if ! command -v gopherage >/dev/null 2>&1; then
    echo "gopherage not found; installing now"
    go install k8s.io/test-infra/gopherage@v0.0.0-20250108071429-415d758da0fa
  fi
  export PATH=$GOBIN:$GOPATH/bin:$PATH
  cp cover.out "${ARTIFACTS}/cover.out"
  go tool cover -func="${ARTIFACTS}/cover.out" -o "${ARTIFACTS}/cover.txt"
	go tool cover -html="${ARTIFACTS}/cover.out" -o "${ARTIFACTS}/coverage.html"
  gopherage filter --exclude-path="zz_generated,generated\.go" "${ARTIFACTS}/cover.out" > "${ARTIFACTS}/filtered.cov"
  gopherage junit --threshold 0 "cover.out" > "${ARTIFACTS}/junit_coverage.xml"
  echo "Saved JUnit XML test report."
}

goTestFlags=""

##### Create a junit-style XML test report in this directory if set. #####
JUNIT_REPORT_DIR=${JUNIT_REPORT_DIR:-}

# If JUNIT_REPORT_DIR is unset, and ARTIFACTS is set, then have them match.
if [[ -z "${JUNIT_REPORT_DIR:-}" && -n "${ARTIFACTS:-}" ]]; then
  export JUNIT_REPORT_DIR="${ARTIFACTS}"
fi

# go test ${goTestFlags} -race ./cmd/... ./pkg/... | tee ${junit_filename_prefix:+"${junit_filename_prefix}.stdout"} | grep --binary-files=text "${go_test_grep_pattern}"
go test -v -race ./cmd/... ./pkg/... -coverprofile cover.out

produce_junit_xmlreport
