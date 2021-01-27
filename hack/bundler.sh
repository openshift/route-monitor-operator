#!/bin/bash
set -euxo pipefail
cd route-monitor-operator
make packagemanifests
NEW_VERSION=$(ls "$BUNDLE_DIR" | sort -t . -k 3 -g | tail -n 1)
chmod 775 -R $BUNDLE_DIR/$NEW_VERSION
