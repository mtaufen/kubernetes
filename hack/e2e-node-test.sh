#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

KUBE_ROOT=$(dirname "${BASH_SOURCE}")/..
source "${KUBE_ROOT}/hack/lib/init.sh"

focus=${FOCUS:-""}
skip=${SKIP:-""}
report=${REPORT:-"/tmp/"}
artifacts=${ARTIFACTS:-"/tmp/_artifacts"}
remote=${REMOTE:-"false"}
images=${IMAGES:-"e2e-node-containervm-v20160321-image"}
hosts=${HOSTS:-""}
image_project=${IMAGE_PROJECT:-"kubernetes-node-e2e-images"}
instance_prefix=${INSTANCE_PREFIX:-"test"}
cleanup=${CLEANUP:-"true"}
delete_instances=${DELETE_INSTANCES:-"true"}
run_until_failure=${RUN_UNTIL_FAILURE:-"false"}

ginkgo=$(kube::util::find-binary "ginkgo")
if [[ -z "${ginkgo}" ]]; then
  echo "You do not appear to have ginkgo built. Try 'make WHAT=vendor/github.com/onsi/ginkgo/ginkgo'"
  exit 1
fi

if [ "$REMOTE" = true ] ; then
  if [ ! -d "${artifacts}" ]; then
    echo "Creating artifacts directory at ${artifacts}"
    mkdir -p ${artifacts}
  fi
  echo "Test artifacts will be written to ${artifacts}"

  z="$(gcloud config list compute/zone 2>/dev/null | grep zone)"
  zone_regex="zone\s*=\s*(.+)"
  if [[ $z =~  $zone_regex ]]; then
    zone="${BASH_REMATCH[1]}"
  else
    echo "Could not find compute zone from running 'gcloud config list compute/zone'"
    exit 1
  fi

  # Get the local project name
  p="$(gcloud config list project 2>/dev/null | grep project)"
  project_regex="project\s*=\s*(.+)"
  if [[ $p =~ $project_regex ]]; then
    project="${BASH_REMATCH[1]}"
  else
    echo "Could not find compute zone from running 'gcloud config list project'"
    exit 1
  fi

  IFS=',' read -ra IM <<< "$images"
  images=""
  for i in "${IM[@]}"; do
    if [[ $(gcloud compute instances list "${instance_prefix}-$i" | grep $i) ]]; then
      if [[ $hosts != "" ]]; then
        hosts="$hosts,"
      fi
      echo "Reusing host ${instance_prefix}-$i"
      hosts="${hosts}${instance_prefix}-${i}"
    else
      if [[ $images != "" ]]; then
        images="$images,"
      fi
      images="$images$i"
    fi
  done

  echo "Running tests remotely using"
  echo "Project: $project"
  echo "Image Project: $image_project"
  echo "Zone: $zone"
  echo "Images: $images"
  echo "Hosts: $hosts"

  ginkgoflags=""
  if [[ $focus != "" ]]; then
     ginkgoflags="$ginkgoflags -focus=$focus "
  fi

  if [[ $skip != "" ]]; then
     ginkgoflags="$ginkgoflags -skip=$skip "
  fi

  if [[ $run_until_failure != "" ]]; then
     ginkgoflags="$ginkgoflags -untilItFails=$run_until_failure "
  fi

  go run test/e2e_node/runner/run_e2e.go  --logtostderr --vmodule=*=2 --ssh-env="gce" \
    --zone="$zone" --project="$project"  \
    --hosts="$hosts" --images="$images" --cleanup="$cleanup" \
    --results-dir="$artifacts" --ginkgo-flags="$ginkgoflags" \
    --image-project="$image_project" --instance-name-prefix="$instance_prefix" --setup-node="true" \
    --delete-instances="$delete_instances"

else
  # Test using the host the script was run on
  # Provided for backwards compatibility
  "${ginkgo}" --focus=$focus --skip=$skip "${KUBE_ROOT}/test/e2e_node/" --report-dir=${report} \
    -- --alsologtostderr --v 2 --node-name $(hostname) --build-services=true --start-services=true --stop-services=true
fi


exit $?
