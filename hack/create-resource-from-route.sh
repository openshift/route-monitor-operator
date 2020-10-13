#!/bin/bash

set -e


for bin in kustomize jq yq oc; do
	if [[ ! $(which ${bin}) ]]; then
		echo "required binary ${bin} does not exist on machine, exiting"
		exit 1
	fi
done

NAME="$1"
NAMESPACE="${2:-$(oc project -q)}"

if [[ -z ${NAME} ]]; then
	echo "a 'NAME' of the resource is required for this script"
	exit 1
fi

# in case an input comes from a 'oc get route -oname'
if [[ ${NAME} == *"/"* ]]; then
	NAME=${NAME#route.route.openshift.io/}
fi


# if you don't want to install yq, you can use the snippet from https://gist.github.com/mboersma/1329669#gistcomment-2691156
# yq here is only to convert yaml to json
kustomize build config/samples/ | \
	yq -j r - | \
	jq \
	  --arg name "${NAME}" \
	  --arg namespace "${NAMESPACE}" \
	  '.spec.route.name = $name | .spec.route.namespace = $namespace' | \
        oc apply -f -
