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

PKG=sigs.k8s.io/ibm-powervs-block-csi-driver
GIT_COMMIT?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
REGISTRY?=gcr.io/k8s-staging-cloud-provider-ibm
IMG?=ibm-powervs-block-csi-driver
TAG?=$(GIT_COMMIT)
LDFLAGS?="-X ${PKG}/pkg/driver.driverVersion=${TAG} -X ${PKG}/pkg/driver.gitCommit=${GIT_COMMIT} -X ${PKG}/pkg/driver.buildDate=${BUILD_DATE} -s -w"
RELEASE_TAG ?= $(shell git describe --abbrev=0 2>/dev/null)
PULL_BASE_REF ?= $(RELEASE_TAG) # PULL_BASE_REF will be provided by Prow
RELEASE_ALIAS_TAG ?= $(PULL_BASE_REF)

GO111MODULE=on
GOPROXY=direct
GOPATH=$(shell go env GOPATH)
GOOS=$(shell go env GOOS)
GOBIN=$(shell pwd)/bin

.EXPORT_ALL_VARIABLES:

bin:
	@mkdir -p $@

.PHONY: driver
driver: | bin
	CGO_ENABLED=0 go build -ldflags ${LDFLAGS} -o bin/ibm-powervs-block-csi-driver ./cmd/

.PHONY: node-update-controller
node-update-controller: | bin
	CGO_ENABLED=0 go build -ldflags ${LDFLAGS} -o bin/node-update-controller ./adhoc-controllers/

.PHONY: test
test:
	./hack/test.sh

.PHONY: image
image:
	docker build -t $(REGISTRY)/$(IMG):$(TAG) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		. --target centos-base

.PHONY: push
push:
	docker push $(REGISTRY)/$(IMG):$(TAG)

build-and-push-multi-arch: init-buildx
	{                                                                       \
	set -e ;                                                                \
	BUILDX_NO_DEFAULT_ATTESTATIONS=1 docker buildx build \
		--builder multiarch-multiplatform-builder \
		--platform linux/amd64,linux/ppc64le \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(REGISTRY)/$(IMG):$(TAG) \
		--push . --target centos-base; \
	}

.PHONY: release-alias-tag
release-alias-tag: # Adds the tag to the last build tag.
	gcloud container images add-tag -q $(REGISTRY)/$(IMG):$(TAG) $(REGISTRY)/$(IMG):$(RELEASE_ALIAS_TAG)

.PHONY: release-staging
release-staging: ## Builds and push container images to the staging image registry.
	$(MAKE) build-and-push-multi-arch
	$(MAKE) release-alias-tag

.PHONY: clean
clean:
	rm -rf bin/*

bin/mockgen: | bin
	go install go.uber.org/mock/mockgen@v0.6.0

bin/ginkgo: | bin
	go install github.com/onsi/ginkgo/v2/ginkgo@v2.32.0

bin/golangci-lint: | bin
	echo "Installing golangci-lint..."
	curl -sSfL https://golangci-lint.run/install.sh | sh -s v2.11.2

bin/govulncheck: | bin
	echo "Installing govulncheck..."
	go install golang.org/x/vuln/cmd/govulncheck@v1.7.0

mockgen: bin/mockgen
	./hack/update-gomock

.PHONY: verify
verify: bin/golangci-lint
	echo "verifying and linting files ..."
	./hack/verify-all
	echo "Congratulations! All Go source files have been linted."

.PHONY: govulncheck
govulncheck: bin/govulncheck
	$(GOBIN)/govulncheck ./...

.PHONY: verify-vendor
test: verify-vendor
verify: verify-vendor
verify-vendor:
	@ echo; echo "### $@:"
	@ ./hack/verify-vendor.sh

init-buildx:
	# Ensure we use a builder that can leverage it (the default on linux will not)
	-docker buildx rm multiarch-multiplatform-builder
	docker buildx create --use --name=multiarch-multiplatform-builder
	docker run --rm --privileged tonistiigi/binfmt@sha256:8f58e6214f4cc9dc83ce8f5acad1ece508eb6b20e696a8c1e9f274481982c541 --install amd64,ppc64le # tonistiigi/binfmt:qemu-v10.0.4
	# Register gcloud as a Docker credential helper.
	# Required for "docker buildx build --push".
	gcloud auth configure-docker --quiet

test-integration:
	go test -v -timeout 100m sigs.k8s.io/ibm-powervs-block-csi-driver/tests/it -run ^TestIntegration$

test-e2e: bin/ginkgo
	$(GOBIN)/ginkgo --procs=5 --timeout=100m -v ./tests/e2e/...
