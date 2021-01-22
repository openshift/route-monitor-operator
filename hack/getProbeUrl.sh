#!/bin/bash

# This script extracts the URL that's being monitored by a given either RouteMonitor or ClusterUrlMonitor
# by querying the operator-generated ServiceMonitor

set -euo pipefail

NAMESPACE=$1
NAME=$2
RESOURCE_NAME=$3

SERVICEMONITOR_NAMESPACE="$(oc get $RESOURCE_NAME -n "$NAMESPACE" "$NAME" -o 'jsonpath={.status.serviceMonitorRef.namespace}')"
SERVICEMONITOR_NAME="$(oc get $RESOURCE_NAME -n "$NAMESPACE" "$NAME" -o 'jsonpath={.status.serviceMonitorRef.name}')"

SCHEME="$(oc get servicemonitor -n "$SERVICEMONITOR_NAMESPACE" "$SERVICEMONITOR_NAME" -o 'jsonpath={.spec.endpoints[0].scheme}')"
TARGET="$(oc get servicemonitor -n "$SERVICEMONITOR_NAMESPACE" "$SERVICEMONITOR_NAME" -o 'jsonpath={.spec.endpoints[0].params.target[0]}')"

echo "$SCHEME://$TARGET"
