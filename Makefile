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

PKG=github.com/ppc64le-cloud/powervs-csi-driver
IMAGE?=quay.io/powercloud/powervs-csi-driver
VERSION=v0.0.1
GIT_COMMIT?=$(shell git rev-parse HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${VERSION} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} -s -w"
GO111MODULE=on
GOPROXY=direct
GOPATH=$(shell go env GOPATH)
GOOS=$(shell go env GOOS)
GOBIN=$(shell pwd)/bin
PLATFORM=linux/ppc64le

.EXPORT_ALL_VARIABLES:

.PHONY: bin/powervs-csi-driver
bin/powervs-csi-driver: | bin
	CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -ldflags ${LDFLAGS} -o bin/powervs-csi-driver ./cmd/

.PHONY: test
test:
	go test -v -race ./cmd/... ./pkg/...

.PHONY: image-release
image-release:
	docker buildx build -t $(IMAGE):$(VERSION) . --target debian-base

.PHONY: image
image:
	docker build -t $(IMAGE):latest . --target debian-base

.PHONY: push-release
push-release:
	docker push $(IMAGE):$(VERSION)

.PHONY: push
push:
	docker push $(IMAGE):latest

.PHONY: clean
clean:
	rm -rf bin/*
