# Image URL to use all building/pushing image targets
IMG ?= controller:v0.2.2
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.29.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
SHELL = /usr/bin/env bash

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions.
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*##/ { printf "  \033[36m%%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST) 

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

.PHONY: run
run: fmt vet ## Run a controller from your host.
	go run ./main.go

##@ Build

.PHONY: build
build: fmt vet ## Build manager binary.
	go build -o bin/manager main.go

.PHONY: plugin
plugin: fmt vet ## Build kubectl-forensic plugin.
	go build -o bin/kubectl-forensic cmd/kubectl-forensic/main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build --build-arg TARGETARCH=$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/') -t ${IMG} .

.PHONY: kind-load
kind-load: ## Load docker image into kind cluster (default name 'kind').
	docker save -o controller.tar ${IMG}
	kind load image-archive controller.tar
	rm controller.tar

##@ Deployment

.PHONY: deploy
deploy: ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	kubectl apply -f config/rbac/rbac.yaml
	kubectl apply -f deploy/manager.yaml

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster.
	kubectl delete -f deploy/manager.yaml
	kubectl delete -f config/rbac/rbac.yaml
