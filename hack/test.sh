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
set -o nounset
set -o pipefail

junitFilenamePrefix() {
  if [[ -z "${JUNIT_REPORT_DIR}" ]]; then
    echo ""
    return
  fi
  mkdir -p "${JUNIT_REPORT_DIR}"
  DATE=$( date +%s | base64 | head -c 15 )
  echo "${JUNIT_REPORT_DIR}/junit_$DATE"
}

function produce_junit_xmlreport {
  local -r junit_filename_prefix=$1
  if [[ -z "${junit_filename_prefix}" ]]; then
    return
  fi

  local junit_xml_filename
  junit_xml_filename="${junit_filename_prefix}.xml"

  if ! command -v gotestsum >/dev/null 2>&1; then
    echo "gotestsum not found; installing now"
    go install gotest.tools/gotestsum@v1.12.0
  fi
  export PATH=$GOBIN:$GOPATH/bin:$PATH
  gotestsum --junitfile "${junit_xml_filename}" --raw-command cat "${junit_filename_prefix}"*.stdout
  echo "Saved JUnit XML test report to ${junit_xml_filename}"
}

goTestFlags=""

##### Create a junit-style XML test report in this directory if set. #####
JUNIT_REPORT_DIR=${JUNIT_REPORT_DIR:-}

# If JUNIT_REPORT_DIR is unset, and ARTIFACTS is set, then have them match.
if [[ -z "${JUNIT_REPORT_DIR:-}" && -n "${ARTIFACTS:-}" ]]; then
  export JUNIT_REPORT_DIR="${ARTIFACTS}"
fi

# Used to filter verbose test output.
go_test_grep_pattern=".*"

if [[ -n "${JUNIT_REPORT_DIR}" ]] ; then
  goTestFlags+="-v "
  goTestFlags+="-json "
  # Show only summary lines by matching lines like "status package/test"
  go_test_grep_pattern="^[^[:space:]]\+[[:space:]]\+[^[:space:]]\+/[^[[:space:]]\+"
fi

junit_filename_prefix=$(junitFilenamePrefix)

go test ${goTestFlags} -race ./cmd/... ./pkg/... | tee ${junit_filename_prefix:+"${junit_filename_prefix}.stdout"} | grep --binary-files=text "${go_test_grep_pattern}"

produce_junit_xmlreport "${junit_filename_prefix}"
