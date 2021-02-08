#!/bin/bash

set -euo pipefail

export KUBECONFIG=${KUBECONFIG:-$HOME/.kube/config}
export NAMESPACE=${NAMESPACE:-openshift-route-monitor-operator}
export IMAGE_NAME=route-monitor-operator
export KUBECTL=oc

SKIP=${1:-""}

function buildImage {
  echo -e "\n\nSTARTING BUILD\n\n"
  oc adm new-project "$NAMESPACE" || true
  oc new-build --binary --strategy=docker --name "$IMAGE_NAME" -n "$NAMESPACE" || true
  oc start-build -n "$NAMESPACE" "$IMAGE_NAME" --from-dir . -F
  oc set image-lookup -n "$NAMESPACE" "$IMAGE_NAME"
}

function deployOperator {
  echo -e "\n\nDEPLOYING OPERATOR\n\n"
  oc delete deployment "route-monitor-operator-controller-manager" -n "$NAMESPACE" || true

  # Override namespace in all objects
  cp -r config{,.bak}
  find config -type f | xargs sed -i "s/openshift-route-monitor-operator/$NAMESPACE/g"
 
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
      echo " [done]"
      return 0
    fi
  done
  if [[ $i -eq 20 ]]; then
    return 1
  fi
}

function runTests {
  echo -e "\n\nRUNNING INTEGRATION TESTS\n\n"
	if go test ./int -count=1; then
    echo -e "\n\nINTEGRATION TEST SUCCESSFUL!\n\n"
    return 0
  fi
  echo -e "\n\nINTEGRATION TEST FAILED!\n\n"
  return 1
}

function cleanup {
  echo -e "\n\nCLEANING UP\n\n"
  oc delete namespace "$NAMESPACE" || true
  if [[ -d config.bak ]]; then
    rsync -v config{.bak/*,}
    rm -r config.bak
  fi

}

if [[ "$SKIP" != "--skip-test" ]]; then
  trap cleanup EXIT
fi

if [[ "$SKIP" != "--skip-build" ]]; then
  buildImage
fi

deployOperator

waitForDeployment

if [[ "$SKIP" != "--skip-test" ]]; then
  runTests
fi
