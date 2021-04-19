#!/bin/bash

# AppSRE team CD

set -exv

CURRENT_DIR=$(dirname "$0")

BASE_IMG="route-monitor-operator"
QUAY_IMAGE="${QUAY_IMAGE:-quay.io/app-sre/${BASE_IMG}}"
IMG="${BASE_IMG}:latest"

REPO_ROOT=$(git rev-parse --show-toplevel)
source $REPO_ROOT/boilerplate/_lib/common.sh

GIT_HASH=$(git rev-parse --short=7 HEAD)
CURRENT_COMMIT=$GIT_HASH

# build the image
OPERATOR_IMAGE_URI=${QUAY_IMAGE}:${GIT_HASH}
REGISTRY_IMAGE="${REGISTRY_IMAGE:-${QUAY_IMAGE}-registry}"


# Don't rebuild the image if it already exists in the repository
if image_exists_in_repo "${OPERATOR_IMAGE_URI}"; then
    echo "Skipping operator image build/push"
else
    # build and push the operator image
    BUILD_CMD="docker build" IMG="$OPERATOR_IMAGE_URI" make docker-build
    if [[ ${DRY_RUN} != 'y' ]] ; then
      skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
        "docker-daemon:${OPERATOR_IMAGE_URI}" \
        "docker://${OPERATOR_IMAGE_URI}"
    fi
fi

for channel in staging production; do
  # If the catalog image already exists, short out
  if image_exists_in_repo "${REGISTRY_IMAGE}:${channel}-${CURRENT_COMMIT}"; then
    echo "Catalog image ${REGISTRY_IMAGE}:${channel}-${CURRENT_COMMIT} already "
    echo "exists. Assuming this means the saas bundle work has also been done "
    echo "properly. Nothing to do!"
  else
    # build the CSV and create & push image catalog for the appropriate channel
    REGISTRY_IMAGE=$REGISTRY_IMAGE "$CURRENT_DIR"/app_sre_create_image_catalog.sh $channel "$QUAY_IMAGE"
  fi
done
