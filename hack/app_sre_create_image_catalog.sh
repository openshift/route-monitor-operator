#!/bin/bash

set -exv

# prefix var with _ so we don't clober the var used during the Make build
# it probably doesn't matter but we can change it later.
_OPERATOR_NAME="route-monitor-operator"

BRANCH_CHANNEL="$1"
QUAY_IMAGE="$2"

GIT_HASH=$(git rev-parse --short=7 HEAD)
GIT_COMMIT_COUNT=$(git rev-list $(git rev-list --max-parents=0 HEAD)..HEAD --count)

# clone bundle repo
SAAS_OPERATOR_DIR="saas-route-monitor-operator-bundle"
BUNDLE_DIR="$SAAS_OPERATOR_DIR/route-monitor-operator/"

rm -rf "$SAAS_OPERATOR_DIR"

git clone \
    --branch "$BRANCH_CHANNEL" \
    https://app:"${APP_SRE_BOT_PUSH_TOKEN}"@gitlab.cee.redhat.com/service/saas-route-monitor-operator-bundle.git \
    "$SAAS_OPERATOR_DIR"

# remove any versions more recent than deployed hash
REMOVED_VERSIONS=""
if [[ "$REMOVE_UNDEPLOYED" == true ]]; then
    DEPLOYED_HASH=$(
        curl -s "https://gitlab.cee.redhat.com/service/app-interface/raw/master/data/services/osd-operators/cicd/saas/saas-${_OPERATOR_NAME}.yaml" | \
            docker run --rm -i quay.io/app-sre/yq yq r - "resourceTemplates[*].targets(namespace.\$ref==/services/osd-operators/namespaces/hivep01ue1/${_OPERATOR_NAME}.yml).ref"
    )

    delete=false
    # Sort based on commit number
    for version in $(ls $BUNDLE_DIR | sort -t . -k 3 -g); do
        # skip if not directory
        [ -d "$BUNDLE_DIR/$version" ] || continue

        if [[ "$delete" == false ]]; then
            short_hash=$(echo "$version" | cut -d- -f2)

            if [[ "$DEPLOYED_HASH" == "${short_hash}"* ]]; then
                delete=true
            fi
        else
            rm -rf "${BUNDLE_DIR:?BUNDLE_DIR var not set}/$version"
            REMOVED_VERSIONS="$version $REMOVED_VERSIONS"
        fi
    done
fi

# generate bundle
PREV_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)

# build the registry image
REGISTRY_IMG="quay.io/app-sre/route-monitor-operator-registry"

# Build an image locally that has all tools we need
#
# TODO: This section is done as in the pipeline stuff is running on the Jenkins slave
#       that doesn't have `operator-sdk >=1` and `kustomize`
#       so we build a container with those tools and run all operations inside
#
# what this section does in general is `make packagemanifests`
docker rm route-monitor-operator-pipeline || true
docker build -f hack/pipeline.dockerfile -t pipelinebuilder:latest .
docker run  \
-e CHANNELS=$BRANCH_CHANNEL \
-e IMG=$QUAY_IMAGE:$GIT_HASH \
-e PREV_VERSION=$PREV_VERSION \
--name route-monitor-operator-pipeline \
pipelinebuilder:latest
docker cp route-monitor-operator-pipeline:/pipeline/route-monitor-operator/packagemanifests .
docker rm route-monitor-operator-pipeline
rsync -a packagemanifests/* $BUNDLE_DIR/
rm -rf packagemanifests

BUNDLE_DIR=$BUNDLE_DIR BUNDLE_IMG="${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" make packagemanifests-build

pushd $SAAS_OPERATOR_DIR

git add .

MESSAGE="add version $GIT_COMMIT_COUNT-$GIT_HASH

replaces $PREV_VERSION
removed versions: $REMOVED_VERSIONS"

git commit -m "$MESSAGE"
popd

NEW_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)

if [ "$NEW_VERSION" = "$PREV_VERSION" ]; then
    # stopping script as that version was already built, so no need to rebuild it
    exit 0
fi

if [[ "$DRY_RUN" == "y" ]]; then
  exit 0
fi


pushd $SAAS_OPERATOR_DIR
  git push origin "$BRANCH_CHANNEL"
popd


# push image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-${GIT_HASH}"
