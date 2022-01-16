# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Version of local tools to be downloaded
KIND_VERSION 	:= v0.11.1
KUBECTL_VERSION := v1.23.1

# Name of the local Kubernetes cluster. Set it to avoid conflicting with other Kind clusters.
export KIND_CLUSTER_NAME	= grace

# Image used by the local Kubernetes cluster. Right now we use Kubernetes 1.23.
export KIND_IMAGE           = kindest/node:v1.23.0@sha256:49824ab1727c04e56a21a5d8372a402fcd32ea51ac96a2706a12af38934f81ac

# This is the default Makefile command. It is executed 'make' is run without
# arguments.
.PHONY: all
all: docker-init docker-build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'.
.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Build

.PHONY: build
build: ## Build binary
	CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -ldflags="-w -s" -o bin/grace github.com/fernandrone/grace/cmd/grace

.PHONY: lint
lint: ## Lint files
	golangci-lint run

.PHONY: deps
deps: ## Download and update dependencies
	go get -u ./...
	go mod tidy
	go mod vendor


##@ Test

.PHONY: test-docker
test-docker: ## Test against docker containers
	./hack/run-test-containers.sh

.PHONY: test-kubernetes
test-kubernetes: kind ## Test against a local kuberenetes cluster
	@if [ "$$(docker inspect -f '{{.State.Running}}' $(KIND_CLUSTER_NAME)-control-plane 2>/dev/null || true)" != 'true' ]; then \
		$(KIND) create cluster --name=$(KIND_CLUSTER_NAME) --config=kind/config.yml	--image=$(KIND_IMAGE) ;\
	else \
		$(MAKE) kubectx ;\
	fi ;\
	$(KUBECTL) apply --wait -n default -f hack/k8s/ ;\
	$(GRACE) pod/trapper-exec pod/trapper-shell ;\
	$(KIND) delete cluster --name=$(KIND_CLUSTER_NAME) ;\

.SILENT:
.PHONY: kubectx
kubectx: kubectl # Set kubectl context to local Kubernetes cluster
	$(KUBECTL) config use-context kind-$(KIND_CLUSTER_NAME)

KIND = $(shell pwd)/bin/kind
.PHONY: kind
kind: bin # Download Kind if necessary
	@[ -f $(KIND) ] || { \
		curl -Lo $(KIND) https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(OS)-amd64 ;\
	}
	@[ -x $(KIND) ] || { \
  		chmod +x $(KIND) ;\
	}

KUBECTL = $(shell pwd)/bin/kubectl
.PHONY: kubectl
kubectl: bin # Download Kubectl if necessary
	@[ -f $(KUBECTL) ] || { \
  		curl -Lo $(KUBECTL) "https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(OS)/amd64/kubectl" ;\
	}
	@[ -x $(KUBECTL) ] || { \
  		chmod +x $(KUBECTL) ;\
	}

GRACE = $(shell pwd)/bin/grace
.PHONY: grace
grace: bin # Build grace if necessary
	@[ -f $(GRACE) ] || { \
  		$$(MAKE) build ;\
	}

BIN = $(shell pwd)/bin
.PHONY: bin
bin: # Create bin folder
	@[ -d $(BIN) ] || { \
  		mkdir -p $(shell pwd)/bin ;\
	}
