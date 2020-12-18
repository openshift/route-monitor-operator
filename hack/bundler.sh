#!/bin/bash
git clone https://github.com/openshift/route-monitor-operator.git
cd route-monitor-operator
make bundle
chmod 777 -R bundle/
