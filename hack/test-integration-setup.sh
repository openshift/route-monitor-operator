#!/bin/bash

set -euo pipefail

KUBEUSER=${KUBEUSER:-kubeadmin}

oc patch configs.imageregistry.operator.openshift.io/cluster --patch '{"spec":{"defaultRoute":true}}' --type=merge
REGISTRY=$(oc get route default-route -n openshift-image-registry -o json | jq -r .spec.host)
IMAGE_NAME=route-monitor-operator
IMAGE=$REGISTRY/openshift-monitoring/$IMAGE_NAME
podman build . -t "$REGISTRY/openshift-monitoring/route-monitor-operator"
TOKEN=$(oc whoami -t)
podman login "$REGISTRY" -u "$KUBEUSER" -p "$TOKEN" --tls-verify=false
podman push "$IMAGE" --tls-verify=false
oc create imagestream "$IMAGE_NAME" -n openshift-monitoring || true
oc set image-lookup -n openshift-monitoring "$IMAGE_NAME"
IMG=$IMAGE_NAME make deploy
