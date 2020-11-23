#!/bin/bash

set -euo pipefail

export KUBEUSER=${KUBEUSER:-kubeadmin}
export IMAGE_NAME=route-monitor-operator

function ensureRegistryExposed {
  oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge
}

function pushImage {
  REGISTRY=$(oc get route default-route -n openshift-image-registry -o 'jsonpath={.spec.host}')
  IMAGE=$REGISTRY/openshift-monitoring/$IMAGE_NAME
  podman build . -t "$REGISTRY/openshift-monitoring/route-monitor-operator"
  TOKEN=$(oc whoami -t)
  podman login "$REGISTRY" -u "$KUBEUSER" -p "$TOKEN" --tls-verify=false
  podman push "$IMAGE" --tls-verify=false
}

function ensureImagestreamExists {
  oc create imagestream "$IMAGE_NAME" -n openshift-monitoring || true
  oc set image-lookup -n openshift-monitoring "$IMAGE_NAME"
}

function deployOperator {
  IMG=$IMAGE_NAME make deploy
}


ensureRegistryExposed
pushImage
ensureImagestreamExists
deployOperator
