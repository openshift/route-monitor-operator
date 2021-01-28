# Current Operator version
VERSION_MAJOR=0
VERSION_MINOR=1
COMMIT_NUMBER=$(shell git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$$"`..HEAD --count)
CURRENT_COMMIT=$(shell git rev-parse --short=7 HEAD)
OPERATOR_VERSION=$(VERSION_MAJOR).$(VERSION_MINOR).$(COMMIT_NUMBER)-$(CURRENT_COMMIT)
KUBECTL ?= kubectl

VERSION ?= $(OPERATOR_VERSION)
PREV_VERSION ?= $(VERSION)
# Default bundle image tag
BUNDLE_IMG ?= controller-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

OPERATOR_SDK ?= operator-sdk

all: manager

TESTS=$(shell go list ./... | grep -v /int | tr '\n' ' ')
# Run tests
test: generate fmt vet manifests
	go test $(TESTS) -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run-verbose: generate fmt vet manifests
	go run ./main.go --zap-log-level=5

# Install CRDs into a cluster
install: manifests kustomize
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete -f -

pre-deploy: kustomize 
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: pre-deploy manifests kustomize
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

# Install CRDs into a cluster
sample-install: manifests kustomize
	$(KUSTOMIZE) build config/samples | $(KUBECTL) apply -f -
#
# Uninstall CRDs into a cluster
sample-uninstall: manifests kustomize
	$(KUSTOMIZE) build config/samples | $(KUBECTL) delete -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

go-generate: mockgen
	go generate ./...

# Build the docker image
docker-build: test
	docker build . -t ${IMG}

# Build the image with podman
podman-build:
	podman build . -t ${IMG}

# Push the image with podman
podman-push:
	podman push ${IMG} --tls-verify=false

test-integration:
	hack/test-integration.sh


# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

mockgen:
ifeq (, $(shell which mockgen))
	@{ \
	set -e ;\
	MOCKGEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$MOCKGEN_TMP_DIR ;\
	go mod init tmp ;\
	GO111MODULE=on go get github.com/golang/mock/mockgen@v1.4.4 ;\
	rm -rf $$MOCKGEN_TMP_DIR ;\
	}
MOCKGEN=$(GOBIN)/mockgen
else
MOCKGEN=$(shell which mockgen)
endif

kustomize:
ifndef KUSTOMIZE
ifeq (, $(shell which kustomize))
	@{ \
	set -e ;\
	KUSTOMIZE_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$KUSTOMIZE_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/kustomize/kustomize/v3@v3.8.8 ;\
	rm -rf $$KUSTOMIZE_GEN_TMP_DIR ;\
	}
KUSTOMIZE=$(GOBIN)/kustomize
else
KUSTOMIZE=$(shell which kustomize)
endif
endif

export YAML_DIRECTORY?=hack/olm-base-resources
export SELECTOR_SYNC_SET_TEMPLATE_DIR?=hack/templates/
GIT_ROOT?=$(shell git rev-parse --show-toplevel 2>&1)

export SELECTOR_SYNC_SET_DESTINATION?=${GIT_ROOT}/hack/olm-registry/olm-artifacts-template.yaml

add-kustomize-data: kustomize
	    rm -rf $(YAML_DIRECTORY)
	    mkdir $(YAML_DIRECTORY)
	    $(KUSTOMIZE) build config/olm/ > $(YAML_DIRECTORY)/00_olm-resources_generated.yaml

.PHONY: generate-syncset
generate-syncset: kustomize add-kustomize-data
	hack/generate-syncset.sh

# Generate bundle manifests and metadata, then validate generated files.
bundle: manifests kustomize
	$(OPERATOR_SDK) generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK) bundle validate ./bundle

# Build the bundle image.
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

packagemanifests: manifests kustomize pre-deploy
	$(OPERATOR_SDK) generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate packagemanifests -q \
		--channel $(CHANNELS) \
		--version $(VERSION) \
		--from-version $(PREV_VERSION) \
		--input-dir $(BUNDLE_DIR) \
		--output-dir $(BUNDLE_DIR) \

packagemanifests-build:
	docker build -f packagemanifests.Dockerfile -t $(BUNDLE_IMG) --build-arg BUNDLE_DIR=$(BUNDLE_DIR) .

syncset-install: 
	oc process --local -f $(SELECTOR_SYNC_SET_DESTINATION) \
			CHANNEL=$(CHANNELS) \
			REGISTRY_IMG=$(REGISTRY_IMG) \
			IMAGE_TAG=$(VERSION) \
		| jq '{"kind": "List", "apiVersion": "v1", "items": .items[].spec.resources}' \
		| kubectl apply -f -

syncset-uninstall:
	oc process --local -f $(SELECTOR_SYNC_SET_DESTINATION)  \
			CHANNEL=$(CHANNELS) \
			REGISTRY_IMG=$(REGISTRY_IMG) \
			IMAGE_TAG=$(VERSION) \
		| jq '{"kind": "List", "apiVersion": "v1", "items": .items[].spec.resources}' \
		| kubectl delete -f -
