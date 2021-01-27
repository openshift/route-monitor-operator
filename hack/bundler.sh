#!/bin/bash
set -euxo pipefail
cd route-monitor-operator
make packagemanifests
# chmod 775 -R $BUNDLE_DIR
