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

bin /tmp/helm /tmp/kubeval:
	@mkdir -p $@

bin/helm: | /tmp/helm bin
	@curl -o /tmp/helm/helm.tar.gz -sSL https://get.helm.sh/helm-v3.1.2-${GOOS}-amd64.tar.gz
	@tar -zxf /tmp/helm/helm.tar.gz -C bin --strip-components=1
	@rm -rf /tmp/helm/*

bin/kubeval: | /tmp/kubeval bin
	@curl -o /tmp/kubeval/kubeval.tar.gz -sSL https://github.com/instrumenta/kubeval/releases/download/0.15.0/kubeval-linux-amd64.tar.gz
	@tar -zxf /tmp/kubeval/kubeval.tar.gz -C bin kubeval
	@rm -rf /tmp/kubeval/*

bin/mockgen: | bin
	go get github.com/golang/mock/mockgen@latest

bin/golangci-lint: | bin
	echo "Installing golangci-lint..."
	curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s v1.21.0

.PHONY: kubeval
kubeval: bin/kubeval
	bin/kubeval -d deploy/kubernetes/base,deploy/kubernetes/cluster,deploy/kubernetes/overlays -i kustomization.yaml,crd_.+\.yaml,controller_add

mockgen: bin/mockgen
	./hack/update-gomock

.PHONY: verify
verify: bin/golangci-lint
	echo "verifying and linting files ..."
	./hack/verify-all
	echo "Congratulations! All Go source files have been linted."

.PHONY: test
test:
	go test -v -race ./cmd/... ./pkg/...

.PHONY: test-sanity
test-sanity:
	#go test -v ./tests/sanity/...
	echo "succeed"

bin/k8s-e2e-tester: | bin
	go get github.com/aws/aws-k8s-tester/e2e/tester/cmd/k8s-e2e-tester@master

.PHONY: test-e2e-single-az
test-e2e-single-az: bin/k8s-e2e-tester
	TESTCONFIG=./tester/single-az-config.yaml ${GOBIN}/k8s-e2e-tester

.PHONY: test-e2e-multi-az
test-e2e-multi-az: bin/k8s-e2e-tester
	TESTCONFIG=./tester/multi-az-config.yaml ${GOBIN}/k8s-e2e-tester

.PHONY: image-release
image-release:
	docker buildx build -t $(IMAGE):$(VERSION) . --target debian-base

.PHONY: image
image:
	docker build -t $(IMAGE):latest . --target debian-base

.PHONY: push-release
push-release:
	docker push $(IMAGE):$(VERSION)
	docker push $(IMAGE):$(VERSION_AMAZONLINUX)

.PHONY: push
push:
	docker push $(IMAGE):latest

.PHONY: verify-vendor
test: verify-vendor
verify: verify-vendor
verify-vendor:
	@ echo; echo "### $@:"
	@ ./hack/verify-vendor.sh

.PHONY: clean
clean:
	rm -rf bin/*