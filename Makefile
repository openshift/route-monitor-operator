FIPS_ENABLED=true
include boilerplate/generated-includes.mk

KUBECTL ?= kubectl

# for boilerplate
OPERATOR_NAME=route-monitor-operator
MAINPACKAGE=.
TESTTARGETS=$(shell ${GOENV} go list -e ./... | egrep -v "/(vendor)/" | grep -v /int)

# need to override boilerplate targets which are not working on this operator
# op-generate openapi-generate: ;

VERSION ?= $(OPERATOR_VERSION)
PREV_VERSION ?= $(VERSION)

IMAGE_TAG_BASE ?= $(OPERATOR_NAME)

# Default bundle image tag
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:$(VERSION)
# Options for 'bundle-build'
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
# CRD_OPTIONS ?= "crd:crdVersions=v1,trivialVersions=true,preserveUnknownFields=false"
CRD_OPTIONS ?= "crd"

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
test: generate fmt vet
	go test $(TESTS) -coverprofile cover.out

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet
	go run ./main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run-verbose: generate fmt vet
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
manifests: controller-gen kustomize yq
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(KUSTOMIZE) build config/default | $(YQ) -s '"deploy/" + .metadata.name + "." + .kind + ".yaml"'


# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
# generate: mockgen controller-gen manifests
# 	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

test-integration:
	hack/test-integration.sh

# CONTROLLER_GEN := $(shell pwd)/bin/controller-gen
# controller-gen: ## Download controller-gen locally if necessary.
# ifeq (,$(wildcard $(CONTROLLER_GEN)))
# 	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.8.0)
# endif

YQ := $(shell pwd)/bin/yq
yq: ## Download yq locally if necessary.
ifeq (,$(wildcard $(YQ)))
	$(call go-get-tool,$(YQ),github.com/mikefarah/yq/v4@v4.27.5)
endif

MOCKGEN := $(shell pwd)/bin/mockgen
mockgen: ## Download kustomize locally if necessary.
ifeq (,$(wildcard $(MOCKGEN)))
	$(call go-get-tool,$(MOCKGEN),github.com/golang/mock/mockgen@v1.4.4)
endif

KUSTOMIZE := $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
ifeq (,$(wildcard $(KUSTOMIZE)))
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v4@v4.5.7)
endif

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath Makefile))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
export GOBIN=$(PROJECT_DIR)/bin ;\
go install $(2) ;\
echo "installed at $(GOBIN)" ;\
rm -rf $$TMP_DIR ;\
}
endef
# from https://sdk.operatorframework.io/docs/upgrading-sdk-version/v1.6.1/#gov2-gov3-ansiblev1-helmv1-add-opm-and-catalog-build-makefile-targets
OS = $(shell go env GOOS)
ARCH = $(shell go env GOARCH)

.PHONY: opm
OPM = ./bin/opm
opm:
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.1/$(OS)-$(ARCH)-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif
BUNDLE_IMGS ?= $(BUNDLE_IMG)
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION) ifneq ($(origin CATALOG_BASE_IMG), undefined) FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG) endif
.PHONY: catalog-build
catalog-build: opm
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

.PHONY: catalog-push
catalog-push: ## Push the catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

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

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
