#!/usr/bin/env bash
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

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_ROOT="$(realpath "$(dirname "$(readlink -f "$0")")"/..)"
JOBSET_ASSETS_DIR="${SCRIPT_ROOT}/bindata/assets/jobset-controller-generated"
JOBSET_CONTROLLER_DIR="${JOBSET_CONTROLLER_DIR:-${HOME}/go/src/sigs.k8s.io/jobset}"

JOBSET_RELEASE_TAG="${JOBSET_RELEASE_TAG:-"upstream/main"}" # "v0.7.3"
JOBSET_NAMESPACE="${JOBSET_NAMESPACE:-openshift-jobset-operator}"

if [ ! -d "${JOBSET_CONTROLLER_DIR}" ]; then
  echo "${JOBSET_CONTROLLER_DIR} is not a valid directory" >&2
  exit 1
fi
if [ -d "${JOBSET_ASSETS_DIR}" ];then
  rm -r "${JOBSET_ASSETS_DIR}"
fi
mkdir -p "${JOBSET_ASSETS_DIR}" "${SCRIPT_ROOT}/_tmp"

pushd "${JOBSET_CONTROLLER_DIR}"
  if [ -n "$(git status --porcelain)" ];then
      echo "${JOBSET_CONTROLLER_DIR} is not a clean git directory" >&2
      exit 2
  fi
  # ensure kustomize exists or download it
  make kustomize
  git checkout "${JOBSET_RELEASE_TAG}"
    # backup kustomization.yaml and edit the default values
    pushd "${JOBSET_CONTROLLER_DIR}/config/default"
      cp "${JOBSET_CONTROLLER_DIR}/config/default/kustomization.yaml" "${SCRIPT_ROOT}/_tmp/jobset_kustomization.yaml.bak"
      "${JOBSET_CONTROLLER_DIR}/bin/kustomize" edit set namespace ${JOBSET_NAMESPACE}
    popd
    pushd "${JOBSET_CONTROLLER_DIR}/config/components/manager"
      cp "${JOBSET_CONTROLLER_DIR}//config/components/manager/kustomization.yaml" "${SCRIPT_ROOT}/_tmp/jobset_components_manager_kustomization.yaml.bak"
      "${JOBSET_CONTROLLER_DIR}/bin/kustomize" edit set image controller='${CONTROLLER_IMAGE}:latest'
    popd
    "${JOBSET_CONTROLLER_DIR}/bin/kustomize" build config/default -o "${JOBSET_ASSETS_DIR}"
    # restore back to the original state
    mv "${SCRIPT_ROOT}/_tmp/jobset_kustomization.yaml.bak" "${JOBSET_CONTROLLER_DIR}/config/default/kustomization.yaml"
    mv  "${SCRIPT_ROOT}/_tmp/jobset_components_manager_kustomization.yaml.bak" "${JOBSET_CONTROLLER_DIR}//config/components/manager/kustomization.yaml"
  git checkout -
popd

# post processing
pushd "${JOBSET_ASSETS_DIR}"
# we supply our own config
rm ./v1_configmap_jobset-manager-config.yaml
# we don't need the namespace object
rm ./v1_namespace_openshift-jobset-operator.yaml
popd