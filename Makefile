# Copyright 2026 Red Hat, Inc.
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

DRIVER_NAME   := dra-driver-ovsdpdk
OPERATOR_NAME := ovs-dpdk-operator
MODULE        := github.com/k8snetworkplumbingwg/dra-driver-ovsdpdk

GOLANG_VERSION        ?= 1.26
GOLANGCI_LINT_VERSION ?= v2.7.2
CONTROLLER_GEN_VERSION ?= v0.21.0
MOCKERY_VERSION        ?= v2.53.6

CONTAINER_TOOL      ?= podman
REGISTRY            ?= ghcr.io/k8snetworkplumbingwg
IMAGE_NAME_DRIVER   ?= $(REGISTRY)/$(DRIVER_NAME)
IMAGE_NAME_OPERATOR ?= $(REGISTRY)/$(OPERATOR_NAME)
IMAGE_TAG           ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

GOOS   ?= linux
GOARCH ?= amd64

BIN_DIR := $(CURDIR)/bin

APIS := ovsdpdkdra/v1alpha1 ovsdpdkoperator/v1alpha1

.PHONY: all build binary binary-driver binary-operator check vet lint test coverage generate generate-deepcopy generate-crds generate-crds-driver generate-crds-operator generate-mocks build-image build-image-driver build-image-operator push-image push-image-driver push-image-operator deploy deploy-driver deploy-operator undeploy undeploy-driver undeploy-operator deploy-openshift deploy-openshift-driver undeploy-openshift undeploy-openshift-driver gen-manifests-poc

all: check test build

build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build ./...

binary: binary-driver binary-operator

binary-driver:
	GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "-s -w" \
		-o $(DRIVER_NAME) \
		$(MODULE)/cmd/$(DRIVER_NAME)

binary-operator:
	GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -ldflags "-s -w" \
		-o $(OPERATOR_NAME) \
		$(MODULE)/cmd/$(OPERATOR_NAME)

check: vet lint

GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

$(GOLANGCI_LINT):
	GOBIN=$(BIN_DIR) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

vet:
	go vet ./...

test:
	go test -v -coverprofile=coverage.out ./cmd/... ./pkg/...

coverage: test
	go tool cover -func=coverage.out

# ---- code generation ------------------------------------------------------

CONTROLLER_GEN := $(BIN_DIR)/controller-gen

generate: generate-deepcopy generate-crds generate-mocks

generate-deepcopy: $(CONTROLLER_GEN)
	for api in $(APIS); do \
		rm -f $(CURDIR)/pkg/api/$${api}/zz_generated.deepcopy.go; \
		$(CONTROLLER_GEN) \
			object:headerFile=$(CURDIR)/hack/boilerplate.generatego.txt \
			paths=$(CURDIR)/pkg/api/$${api}/; \
	done

generate-crds: generate-crds-driver generate-crds-operator

generate-crds-driver: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) \
		crd \
		paths=$(CURDIR)/pkg/api/ovsdpdkdra/v1alpha1/ \
		output:crd:dir=$(CURDIR)/deployments/$(DRIVER_NAME)/crds/

generate-crds-operator: $(CONTROLLER_GEN)
	$(CONTROLLER_GEN) \
		crd \
		paths=$(CURDIR)/pkg/api/ovsdpdkoperator/v1alpha1/ \
		output:crd:dir=$(CURDIR)/deployments/$(OPERATOR_NAME)/crds/

$(CONTROLLER_GEN):
	GOBIN=$(BIN_DIR) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)

MOCKERY := $(BIN_DIR)/mockery

generate-mocks: $(MOCKERY)
	$(MOCKERY)

$(MOCKERY):
	GOBIN=$(BIN_DIR) go install github.com/vektra/mockery/v2@$(MOCKERY_VERSION)

build-image: build-image-driver build-image-operator

build-image-driver:
	$(CONTAINER_TOOL) build \
		--platform $(GOOS)/$(GOARCH) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--target driver \
		-t $(IMAGE_NAME_DRIVER):$(IMAGE_TAG) \
		-f $(CURDIR)/Dockerfile \
		$(CURDIR)

build-image-operator:
	$(CONTAINER_TOOL) build \
		--platform $(GOOS)/$(GOARCH) \
		--build-arg GOLANG_VERSION=$(GOLANG_VERSION) \
		--target operator \
		-t $(IMAGE_NAME_OPERATOR):$(IMAGE_TAG) \
		-f $(CURDIR)/Dockerfile \
		$(CURDIR)

push-image: push-image-driver push-image-operator

push-image-driver: build-image-driver
	$(CONTAINER_TOOL) push $(IMAGE_NAME_DRIVER):$(IMAGE_TAG)

push-image-operator: build-image-operator
	$(CONTAINER_TOOL) push $(IMAGE_NAME_OPERATOR):$(IMAGE_TAG)

# ---- cluster deployment ---------------------------------------------------

deploy: deploy-driver deploy-operator

deploy-driver: ## Deploy the driver (vanilla Kubernetes / kind)
	kubectl apply -f $(CURDIR)/deployments/dra-driver-ovsdpdk/crds/
	kubectl kustomize $(CURDIR)/deployments/dra-driver-ovsdpdk/k8s/ | \
		sed 's|IMAGE_DRA_DRIVER|$(IMAGE_NAME_DRIVER):$(IMAGE_TAG)|g' | \
		kubectl apply -f -

deploy-operator:
	kubectl apply -f $(CURDIR)/deployments/ovs-dpdk-operator/crds/
	kubectl kustomize $(CURDIR)/deployments/ovs-dpdk-operator/k8s/ | \
			OVS_DPDK_OPERATOR_IMAGE=$(IMAGE_NAME_OPERATOR):$(IMAGE_TAG) \
			IMAGE_DRA_DRIVER=$(IMAGE_NAME_DRIVER):$(IMAGE_TAG) \
			DRA_DRIVER_NAMESPACE="dra-driver-ovsdpdk" \
		envsubst | \
		kubectl apply -f -

gen-manifests-poc:
	cp $(CURDIR)/deployments/dra-driver-ovsdpdk/crds/* $(CURDIR)/poc-manifests
	kubectl kustomize $(CURDIR)/deployments/dra-driver-ovsdpdk/operator-poc/ | \
		sed 's|IMAGE_DRA_DRIVER|$(IMAGE_NAME_DRIVER):$(IMAGE_TAG)|g' | \
		envsubst > $(CURDIR)/poc-manifests/dra-driver.yaml  \

undeploy: undeploy-driver undeploy-operator

undeploy-driver: ## Remove the driver (vanilla Kubernetes / kind)
	kubectl kustomize $(CURDIR)/deployments/dra-driver-ovsdpdk/k8s/ | \
		kubectl delete --ignore-not-found -f -
	kubectl delete --ignore-not-found -f $(CURDIR)/deployments/dra-driver-ovsdpdk/crds/

undeploy-operator:
	kubectl kustomize $(CURDIR)/deployments/ovs-dpdk-operator/k8s/ | \
		kubectl delete --ignore-not-found -f -
	kubectl delete --ignore-not-found -f $(CURDIR)/deployments/ovs-dpdk-operator/crds/

deploy-openshift: ## Deploy the driver on OpenShift
	kubectl apply -f $(CURDIR)/deployments/dra-driver-ovsdpdk/crds/
	kubectl kustomize $(CURDIR)/deployments/dra-driver-ovsdpdk/openshift/ | \
		sed 's|IMAGE_DRA_DRIVER|$(IMAGE_NAME_DRIVER):$(IMAGE_TAG)|g' | \
		kubectl apply -f -

undeploy-openshift: ## Remove the driver from OpenShift
	kubectl kustomize $(CURDIR)/deployments/dra-driver-ovsdpdk/openshift/ | \
		kubectl delete --ignore-not-found -f -
	kubectl delete --ignore-not-found -f $(CURDIR)/deployments/dra-driver-ovsdpdk/crds/
