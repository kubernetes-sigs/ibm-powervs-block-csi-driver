# Copyright 2019 The Kubernetes Authors.
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

FROM golang:1.14.1-stretch AS builder
WORKDIR /go/src/github.com/ppc64le-cloud/powervs-csi-driver
COPY . .
RUN make

FROM k8s.gcr.io/build-image/debian-base:v2.1.3 AS debian-base
RUN clean-install ca-certificates e2fsprogs mount udev util-linux xfsprogs
COPY --from=builder /go/src/github.com/ppc64le-cloud/powervs-csi-driver/bin/powervs-csi-driver /bin/powervs-csi-driver

ENTRYPOINT ["/bin/powervs-csi-driver"]
