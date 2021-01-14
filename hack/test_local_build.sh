#!/bin/bash

OLM_BUNDLE_IMAGE="quay.io/jamesh/route-monitor-operator-bundle" \
OLM_CATALOG_IMAGE="quay.io/jamesh/route-monitor-operator-catalog" \
CONTAINER_ENGINE="/usr/bin/podman" \
CONTAINER_ENGINE_CONFIG_DIR="$HOME/.docker" \
CURRENT_COMMIT="0492316" \
OPERATOR_VERSION="0.1.134-0492316" \
OPERATOR_NAME="route-monitor-operator" \
OPERATOR_IMAGE="quay.io/jamesh/route-monitor-operator" \
OPERATOR_IMAGE_TAG="v0.1.134-0492316" \
OLM_CHANNEL="alpha" \
./hack/build-opm-catalog.sh
