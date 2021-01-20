#!/bin/bash

set -euo pipefail

export NAMESPACE=${NAMESPACE:-openshift-route-monitor-operator}
export IMAGE_NAME=route-monitor-operator
SKIP=${1:-""}

function buildImage {
  oc create namespace "$NAMESPACE" || true
  oc new-build --binary --strategy=docker --name "$IMAGE_NAME" -n "$NAMESPACE" || true
  oc start-build -n "$NAMESPACE" "$IMAGE_NAME" --from-dir . -F
  oc set image-lookup -n openshift-route-monitor-operator "$IMAGE_NAME"
}

function deployOperator {
  oc delete deployment "route-monitor-operator-controller-manager" -n "$NAMESPACE" || true
  IMG=$IMAGE_NAME make deploy
}

function waitForDeployment {
  echo "Waiting for operator deployment to finish"
  for i in $(seq 1 60); do
    REPLICAS=$(oc get deployment "route-monitor-operator-controller-manager" -n "$NAMESPACE" -o=jsonpath='{.status.readyReplicas}')
    if [[ $REPLICAS -lt 1 ]]; then
      echo -n .
      sleep 1s
    else
      return 0
    fi
  done
  if [[ $i -eq 20 ]]; then
    return 1
  fi
}

if [[ "$SKIP" != "--skip-build" ]]; then
  buildImage
fi
deployOperator
waitForDeployment
