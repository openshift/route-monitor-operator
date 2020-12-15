#!/usr/bin/env python
#
# Generate an operator bundle for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#
# Usage ./hack/generate-operator-bundle.py OUTPUT_DIR PREVIOUS_VERSION GIT_NUM_COMMITS GIT_HASH HIVE_IMAGE
#
# Commit count can be obtained with: git rev-list 9c56c62c6d0180c27e1cc9cf195f4bbfd7a617dd..HEAD --count
# This is the first hive commit, if we tag a release we can then switch to using that tag and bump the base version.

import subprocess
import tempfile
import datetime
import os
import sys
import yaml
import shutil

# This script will append the current number of commits given as an arg
# (presumably since some past base tag), and the git hash arg for a final
# version like: 0.1.189-3f73a592
VERSION_BASE = "0.1"

if len(sys.argv) != 6:
    print("USAGE: %s OUTPUT_DIR PREVIOUS_VERSION GIT_NUM_COMMITS GIT_HASH HIVE_IMAGE" %
          sys.argv[0])
    sys.exit(1)

outdir = sys.argv[1]
prev_version = sys.argv[2]
git_num_commits = sys.argv[3]
git_hash = sys.argv[4]
route_monitor_operator_image = sys.argv[5]

full_version = "%s.%s-%s" % (VERSION_BASE, git_num_commits, git_hash)
print("Generating CSV for version: %s" % full_version)

BUNDLE_BASE_PATH = 'bundle/manifests'
with open("{}/route-monitor-operator.clusterserviceversion.yaml".format(BUNDLE_BASE_PATH), 'r') as stream:
    csv = yaml.safe_load(stream)

if not os.path.exists(outdir):
    os.mkdir(outdir)

version_dir = os.path.join(outdir, full_version)
if not os.path.exists(version_dir):
    os.mkdir(version_dir)


# Update the versions to include git hash:
csv['spec']['replaces'] = "route-monitor-operator.v%s" % prev_version

# Set the CSV createdAt annotation:
now = datetime.datetime.now()
csv['metadata']['annotations']['createdAt'] = now.strftime(
    "%Y-%m-%dT%H:%M:%SZ")

# Write all files aside of the csv to disk


def is_not_csv_file(filename):
    """Retun an indication if the file entered is the clusterserviceversion (csv) file """
    return not filename.endswith('clusterserviceversion.yaml')


only_bundle_files = [f for f in os.listdir(BUNDLE_BASE_PATH)
                     if os.path.isfile(os.path.join(BUNDLE_BASE_PATH, f))
                     and is_not_csv_file(f)]

for f in only_bundle_files:
    src_file = os.path.join(BUNDLE_BASE_PATH, f)
    dst_file = os.path.join(version_dir, f)
    shutil.copyfile(src_file, dst_file)

# Write the CSV to disk:
csv_filename = "route-monitor-operator.v%s.clusterserviceversion.yaml" % full_version
csv_file = os.path.join(version_dir, csv_filename)
with open(csv_file, 'w') as outfile:
    yaml.dump(csv, outfile, default_flow_style=False)
print("Wrote ClusterServiceVersion: %s" % csv_file)
