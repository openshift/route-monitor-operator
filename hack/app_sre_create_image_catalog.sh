#!/bin/bash

set -exv

# prefix var with _ so we don't clober the var used during the Make build
# it probably doesn't matter but we can change it later.
_OPERATOR_NAME="route-monitor-operator"

BRANCH_CHANNEL="$1"
QUAY_IMAGE="$2"

# Build an image locally that has all tools we need
docker build -f hack/pipeline.dockerfile -t pipelinebuilder:latest ./hack/

# generate the bundle folder
#vol=$(generate_mac_mount $(pwd) /root/tmp)
#base_command="docker run --rm --workdir /root/tmp --interactive --volume $(pwd):/root/tmp"

docker run --name route-monitor-operator-pipeline pipelinebuilder:latest

#make bundle KUSTOMIZE="${base_command} --entrypoint kustomize pipelinebuilder:latest" OPERATOR_SDK="${base_command} pipelinebuilder:latest"
#$base_command --entrypoint bash pipelinebuilder:latest chown -R $(id -u) bundle

docker cp route-monitor-operator-pipeline:/pipeline/route-monitor-operator/bundle .
docker rm route-monitor-operator-pipeline
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

./hack/generate-operator-bundle.py \
    "$BUNDLE_DIR" \
    "$PREV_VERSION" \
    "$GIT_COMMIT_COUNT" \
    "$GIT_HASH" \
    "$QUAY_IMAGE:$GIT_HASH"

$base_command --entrypoint bash pipelinebuilder:latest chown -R $(id -u) bundle

NEW_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)

if [ "$NEW_VERSION" = "$PREV_VERSION" ]; then
    # stopping script as that version was already built, so no need to rebuild it
    exit 0
fi

# create package yaml
cat <<EOF > $BUNDLE_DIR/route-monitor-operator.package.yaml
packageName: route-monitor-operator
channels:
- name: $BRANCH_CHANNEL
  currentCSV: route-monitor-operator.v${NEW_VERSION}
EOF

# add, commit & push
pushd $SAAS_OPERATOR_DIR

git add .

MESSAGE="add version $GIT_COMMIT_COUNT-$GIT_HASH

replaces $PREV_VERSION
removed versions: $REMOVED_VERSIONS"

git commit -m "$MESSAGE"
git push origin "$BRANCH_CHANNEL"

popd

# build the registry image
REGISTRY_IMG="quay.io/app-sre/route-monitor-operator-registry"
DOCKERFILE_REGISTRY="Dockerfile.olm-registry"

cat <<EOF > $DOCKERFILE_REGISTRY
FROM quay.io/openshift/origin-operator-registry:4.5

COPY $SAAS_OPERATOR_DIR manifests
RUN initializer --permissive

CMD ["registry-server", "-t", "/tmp/terminate.log"]
EOF

docker build -f $DOCKERFILE_REGISTRY --tag "${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" .

# push image
skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest"

skopeo copy --dest-creds "${QUAY_USER}:${QUAY_TOKEN}" \
    "docker-daemon:${REGISTRY_IMG}:${BRANCH_CHANNEL}-latest" \
    "docker://${REGISTRY_IMG}:${BRANCH_CHANNEL}-${GIT_HASH}"
