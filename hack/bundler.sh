#!/bin/bash
set -euxo pipefail
cd route-monitor-operator
make packagemanifests
# Extract the current version that will be pushed to the packagemanifest
VERSION=$(make printvars  2>&1 \
	| grep '\sVERSION=' \
	| cut --delimiter=' ' --fields=2 \
	| cut --delimiter='=' --fields=2)

# add these resources to the role
kustomize build config/prom-k8s/ > packagemanifests/$VERSION/additional-prom-roles.yaml
chmod 775 -R packagemanifests
