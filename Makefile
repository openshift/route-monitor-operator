# Current Operator version
VERSION_MAJOR=0
VERSION_MINOR=1
COMMIT_NUMBER=$(shell git rev-list `git rev-list --parents HEAD | egrep "^[a-f0-9]{40}$$"`..HEAD --count)
CURRENT_COMMIT=$(shell git rev-parse --short=7 HEAD)
OPERATOR_VERSION=$(VERSION_MAJOR).$(VERSION_MINOR).$(COMMIT_NUMBER)-$(CURRENT_COMMIT)

VERSION ?= $(OPERATOR_VERSION)
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
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests kustomize
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests kustomize
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

# Install CRDs into a cluster
sample-install: manifests kustomize
	$(KUSTOMIZE) build config/samples | kubectl apply -f -
#
# Uninstall CRDs into a cluster
sample-uninstall: manifests kustomize
	$(KUSTOMIZE) build config/samples | kubectl delete -f -

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
podman-build: test
	podman build . -t ${IMG}

# Push the image with podman
podman-push:
	podman push ${IMG} --tls-verify=false

test-integration: podman-build
	hack/test-integration-setup.sh
	go test ./int -count=1 #disable result cache


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

YAML_DIRECTORY?=hack/olm-base-resources
SELECTOR_SYNC_SET_TEMPLATE_DIR?=hack/templates/
GIT_ROOT?=$(shell git rev-parse --show-toplevel 2>&1)

# WARNING: REPO_NAME will default to the current directory if there are no remotes
REPO_NAME?=$(shell basename $$((git config --get-regex remote\.*\.url 2>/dev/null | cut -d ' ' -f2 || pwd) | head -n1 | sed 's|.git||g'))

SELECTOR_SYNC_SET_DESTINATION?=${GIT_ROOT}/hack/olm-registry/olm-artifacts-template.yaml

ADD_KUSTOMIZE_DATA=mkdir ${YAML_DIRECTORY} || ${KUSTOMIZE} build config/olm/ > ${YAML_DIRECTORY}/00_olm-resources_generated.yaml
GEN_SYNCSET=hack/generate_template.py --template-dir ${SELECTOR_SYNC_SET_TEMPLATE_DIR} --yaml-directory ${YAML_DIRECTORY} --destination ${SELECTOR_SYNC_SET_DESTINATION} --repo-name ${REPO_NAME}
.PHONY: generate-syncset
generate-syncset: kustomize
	${ADD_KUSTOMIZE_DATA}; \
	if [ "${IN_CONTAINER}" == "true" ]; then \
		$(CONTAINER_ENGINE) run --rm -v $$(pwd -P):$$(pwd -P) quay.io/bitnami/python:2.7.18 /bin/sh -c "cd $$(pwd); pip install oyaml; $$(pwd)/${GEN_SYNCSET}"; \
	else \
		${GEN_SYNCSET}; \
	fi

# Generate bundle manifests and metadata, then validate generated files.
bundle: manifests kustomize
	$(OPERATOR_SDK) generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK) bundle validate ./bundle

# Build the bundle image.
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
	
packagemanifest: manifests kustomize
	$(OPERATOR_SDK) generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate packagemanifests  -q --channel staging --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK) bundle validate ./bundle

bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .
