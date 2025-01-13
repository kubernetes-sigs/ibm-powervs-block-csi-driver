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

go test -v -race ./cmd/... ./pkg/... -coverprofile cover.out

if [[ -z "${ARTIFACTS}" ]]; then
    mkdir -p test-artifacts
    export ARTIFACTS=./test-artifacts
fi

cp cover.out "${ARTIFACTS}/cover.out"
go tool cover -html="${ARTIFACTS}/cover.out" -o "${ARTIFACTS}/cover.html"
