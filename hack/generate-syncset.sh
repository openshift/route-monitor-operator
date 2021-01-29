#!/bin/bash

set -euxo pipefail

IN_CONTAINER=${IN_CONTAINER:-true}

if [[ $(which podman) ]]; then
	CONTAINER_ENGINE=podman
elif [[ $(which docker) ]]; then
	CONTAINER_ENGINE=docker
fi
# This is a tad ambitious, but it should usually work.
export REPO_NAME=$(git config --get remote.origin.url | sed 's,.*/,,; s/\.git$//')
# If that still didn't work, warn (but proceed)
if [ -z "$REPO_NAME" ]; then
  echo 'Failed to discover repository name! $REPO_NAME not set!'
fi

if [[ "${IN_CONTAINER}" == "true" ]]; then 
  $CONTAINER_ENGINE run --rm \
    -e SELECTOR_SYNC_SET_TEMPLATE_DIR \
    -e YAML_DIRECTORY \
    -e SELECTOR_SYNC_SET_DESTINATION \
    -e REPO_NAME \
    -v "$(pwd -P):$(pwd -P)" \
    quay.io/bitnami/python:2.7.18 /bin/sh \
    -c "cd $(pwd); pip install oyaml; hack/generate-syncset.sh;cat $SELECTOR_SYNC_SET_DESTINATION";
else
  hack/generate_template.py --template-dir "${SELECTOR_SYNC_SET_TEMPLATE_DIR}" --yaml-directory "${YAML_DIRECTORY}" --destination "${SELECTOR_SYNC_SET_DESTINATION}" --repo-name "${REPO_NAME}"
fi
