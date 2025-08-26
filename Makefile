FIPS_ENABLED=true
include boilerplate/generated-includes.mk

KUBECTL ?= kubectl

# for boilerplate
OPERATOR_NAME=route-monitor-operator
MAINPACKAGE=.
TESTTARGETS=$(shell ${GOENV} go list -e ./... | egrep -v "/(vendor)/" | grep -v /int)

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
install:
	$(KUBECTL) apply -f deploy/crds

# Uninstall CRDs from a cluster
uninstall:
	$(KUBECTL) delete -f deploy/crds

pre-deploy:
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: pre-deploy install
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

test-integration:
	hack/test-integration.sh

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

add-kustomize-data:
	rm -rf $(YAML_DIRECTORY)
	mkdir $(YAML_DIRECTORY)
	$(KUSTOMIZE) build config/olm/ > $(YAML_DIRECTORY)/00_olm-resources_generated.yaml

.PHONY: generate-syncset
generate-syncset: add-kustomize-data
	hack/generate-syncset.sh

# Generate bundle manifests and metadata, then validate generated files.
bundle:
	$(OPERATOR_SDK) generate kustomize manifests -q
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK) bundle validate ./bundle

# Build the bundle image.
bundle-build:
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

packagemanifests: pre-deploy
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

.PHONY: kubectl-package
kubectl-package:
ifeq (,$(wildcard bin/kubectl-package))
	mkdir -p bin
	wget https://github.com/package-operator/package-operator/releases/download/v1.9.1/kubectl-package_$(GOOS)_$(GOARCH) -O bin/kubectl-package
	chmod 755 bin/kubectl-package
endif

.PHONY: package
package: kubectl-package
	$(CONTAINER_ENGINE) run --rm -v ${PWD}:/workdir:z quay.io/app-sre/yq:4 '.spec.template.spec.containers[0].image = "$(IMAGE_REGISTRY)/$(IMAGE_REPOSITORY)/$(OPERATOR_NAME):$(OPERATOR_IMAGE_TAG)"' deploy/route-monitor-operator-controller-manager.Deployment.yaml > packaging/06-deploy/route-monitor-operator-controller-manager.Deployment.yaml
	$(CONTAINER_ENGINE) login -u $(QUAY_USER) -p $(QUAY_TOKEN) quay.io
	DOCKER_CONFIG=$(CONTAINER_ENGINE_CONFIG_DIR)/ ./bin/kubectl-package build -t $(IMAGE_REGISTRY)/$(IMAGE_REPOSITORY)/$(OPERATOR_NAME)-hs-package:$(OPERATOR_IMAGE_TAG) --push packaging/
	DOCKER_CONFIG=$(CONTAINER_ENGINE_CONFIG_DIR)/ ./bin/kubectl-package build -t $(IMAGE_REGISTRY)/$(IMAGE_REPOSITORY)/$(OPERATOR_NAME)-hs-package:latest --push packaging/

.PHONY: everything
everything: package build-push
