#!/bin/bash

set -euo pipefail

export KUBECONFIG=${KUBECONFIG:-$HOME/.kube/config}
export NAMESPACE=${NAMESPACE:-openshift-route-monitor-operator}
export IMAGE_NAME=route-monitor-operator
export KUBECTL=oc


function parseArgs {
  SKIP_BUILD=${SKIP_BUILD:-}
  SKIP_CLEANUP=${SKIP_CLEANUP:-}
  SKIP_TEST=${SKIP_TEST:-}

  PARSED_ARGUMENTS=$(getopt -o 'n:' --long 'namespace:,kubeconfig:,skip-test,skip-build,skip-cleanup' -- "$@")
  eval set -- "$PARSED_ARGUMENTS"
  while :
  do
    case "$1" in
      --skip-test)	SKIP_TEST=1; SKIP_CLEANUP=1	; shift   ;;
      --skip-build)	SKIP_BUILD=1 			; shift   ;;
      --skip-cleanup)	SKIP_CLEANUP=1			; shift   ;;
      -n|--namespace)	NAMESPACE="$2"			; shift 2 ;;
      --kubeconfig)	export KUBECONFIG="$2"			; shift 2 ;;
      # -- means the end of the arguments; drop this, and break out of the while loop
      --) shift; break ;;
      # If invalid options were passed, then getopt should have reported an error,
      # which we checked as VALID_ARGUMENTS when getopt was called...
      *) echo "Unexpected option: $1 - this should not happen."
         usage; break;;
    esac
  done
  echo "SKIP_BUILD=${SKIP_BUILD}"
  echo "SKIP_CLEANUP=${SKIP_CLEANUP}"
  echo "KUBECONFIG=${KUBECONFIG}"
  echo "NAMESPACE=${NAMESPACE}"
  echo "IMAGE_NAME=${IMAGE_NAME}"
}

function usage {
  cat <<EOF
  USAGE: $(basename "$0")

  OPTIONS:
  --skip-build skips the 'oc new-build' process (don't use in the first run as there won't be an image
  --skip-cleanup skips cleanup trap, good for testing how the script works / reporduce locally stuff
  --skip-test skip test runs, good if you just want to deploy
  -n|--namespace sets the namespace to use
  --kubeconfig uses this kubeconfig instead of the current one
EOF
}

function buildImage {
  echo -e "\n\nSTARTING BUILD\n\n"
  oc adm new-project "$NAMESPACE" || true
  oc new-build --binary --strategy=docker --name "$IMAGE_NAME" -n "$NAMESPACE" || true
  oc start-build -n "$NAMESPACE" "$IMAGE_NAME" --from-dir . -F
 
  oc set image-lookup -n "$NAMESPACE" "$IMAGE_NAME"
}

function verifyForBuildSuccess {
  local latestJobName phase
  latestJobName="$IMAGE_NAME"-$( oc -n "$NAMESPACE" get buildconfig "$IMAGE_NAME" -ojsonpath='{.status.lastVersion}') 
  phase=$(oc -n "$NAMESPACE" get build "$latestJobName" -ojsonpath='{.status.phase}')
  if [[ $phase != "Complete" ]]; then
	  echo "build was not completed fully, the state was $phase but expected to be 'Complete'"
	  echo "the logs for the failed job are:"
	  oc -n "$NAMESPACE" logs "$latestJobName"
	  return 1
  fi

}

function deployOperator {
  echo -e "\n\nDEPLOYING OPERATOR\n\n"
  oc delete deployment "route-monitor-operator-controller-manager" -n "$NAMESPACE" || true

  # Override namespace in all objects
  cp -r config{,.bak}
  find config -type f -print0 | xargs -0 sed -i "s/openshift-route-monitor-operator/$NAMESPACE/g"
 
  IMG=$IMAGE_NAME make deploy
}

function waitForDeployment {
  echo "Waiting for operator deployment to finish"
  for i in $(seq 1 60); do
    BASE_REPLICAS=$(oc get deployment "route-monitor-operator-controller-manager" -n "$NAMESPACE" -o=jsonpath='{.status.readyReplicas}:{.status.unavailableReplicas}')
    REPLICAS=$(echo "$BASE_REPLICAS" | cut -d':' -f1)
    UN_REPLICAS=$(echo "$BASE_REPLICAS" | cut -d':' -f2)
    if [[ $REPLICAS -lt 1 ]] || [[ $UN_REPLICAS -ge 1 ]] ; then
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
function printOperatorLogs {
	if [[ $(oc -n "$NAMESPACE" get po -lapp="route-monitor-operator" --no-headers | wc -l) == 0 ]];then 
		return
	fi
	podName=$(oc -n "$NAMESPACE" get po -lapp="route-monitor-operator" -ojsonpath='{.items[0].metadata.name}')
	echo -e "\nstatus of the pod\n"
	oc -n "$NAMESPACE" get po "$podName" -ojsonpath='{.status}' | jq
	echo -e "\npod logs\n"
	oc -n "$NAMESPACE" logs "$podName"
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
  # in case the pod logs are not displayed
  printOperatorLogs
  echo -e "\n\nCLEANING UP\n\n"
  oc delete namespace "$NAMESPACE" || true
  if [[ -d config.bak ]]; then
    rm -rf config
    mv config{.bak,}
  fi
}

parseArgs "$@"


if [[ -z $SKIP_CLEANUP ]]; then  
  trap cleanup EXIT
else
  trap printOperatorLogs EXIT
fi

if [[ -z $SKIP_BUILD ]]; then
  buildImage
  verifyForBuildSuccess
fi

deployOperator

waitForDeployment

if [[ -z $SKIP_TEST ]]; then
  runTests
fi
