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
JOBSET_DEPLOY_DIR="${SCRIPT_ROOT}/deploy"
JOBSET_CONTROLLER_DIR="${JOBSET_CONTROLLER_DIR:-${HOME}/go/src/sigs.k8s.io/jobset}"

JOBSET_BRANCH_OR_TAG="${JOBSET_BRANCH_OR_TAG:-"$(cat "${SCRIPT_ROOT}/operand-git-ref")"}"
JOBSET_NAMESPACE="${JOBSET_NAMESPACE:-openshift-jobset-operator}"

# Ensure yq is installed
if ! command -v yq &> /dev/null; then
    echo "yq is not installed. Installing yq..."
    go install -mod=readonly github.com/mikefarah/yq/v4@v4.45.1
fi

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
  GOFLAGS='-mod=readonly' make kustomize

  ORIGINAL_GIT_BRANCH_OR_COMMIT="$(git branch --show-current)"
  if [[ -z "${ORIGINAL_GIT_BRANCH_OR_COMMIT}" ]]; then
      ORIGINAL_GIT_BRANCH_OR_COMMIT="$(git rev-parse HEAD)"
  fi

  git checkout "${JOBSET_BRANCH_OR_TAG}"
    # backup kustomization.yaml and edit the default values
    pushd "${JOBSET_CONTROLLER_DIR}/config/default"
      cp "${JOBSET_CONTROLLER_DIR}/config/default/kustomization.yaml" "${SCRIPT_ROOT}/_tmp/jobset_kustomization.yaml.bak"
      sed -i 's!#- webhookcainjection_patch.yaml!- webhookcainjection_patch.yaml!' "${JOBSET_CONTROLLER_DIR}/config/default/kustomization.yaml"
      "${JOBSET_CONTROLLER_DIR}/bin/kustomize" edit set namespace ${JOBSET_NAMESPACE}
      "${JOBSET_CONTROLLER_DIR}/bin/kustomize" edit remove resource "../components/internalcert"
      "${JOBSET_CONTROLLER_DIR}/bin/kustomize" edit add resource "../components/certmanager"
    popd
    pushd "${JOBSET_CONTROLLER_DIR}/config/components/manager"
      cp "${JOBSET_CONTROLLER_DIR}//config/components/manager/kustomization.yaml" "${SCRIPT_ROOT}/_tmp/jobset_components_manager_kustomization.yaml.bak"
      "${JOBSET_CONTROLLER_DIR}/bin/kustomize" edit set image controller='${CONTROLLER_IMAGE}:latest'
    popd
    "${JOBSET_CONTROLLER_DIR}/bin/kustomize" build config/default -o "${JOBSET_ASSETS_DIR}"
    # restore back to the original state
    mv "${SCRIPT_ROOT}/_tmp/jobset_kustomization.yaml.bak" "${JOBSET_CONTROLLER_DIR}/config/default/kustomization.yaml"
    mv  "${SCRIPT_ROOT}/_tmp/jobset_components_manager_kustomization.yaml.bak" "${JOBSET_CONTROLLER_DIR}//config/components/manager/kustomization.yaml"
  git checkout "${ORIGINAL_GIT_BRANCH_OR_COMMIT}"
popd

# post processing
pushd "${JOBSET_ASSETS_DIR}"
  # we don't need the namespace object
  rm ./v1_namespace_openshift-jobset-operator.yaml
  # we supply our own config
  rm ./v1_configmap_jobset-manager-config.yaml

  # mirror operand RBAC to the operator
  rm -f "${JOBSET_DEPLOY_DIR}/06_operand_clusterrole.yaml"
  rm -f "${JOBSET_DEPLOY_DIR}/08_operand_role.yaml"

cat >"${JOBSET_DEPLOY_DIR}/06_operand_clusterrole.yaml" <<EOL
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: jobset-operand
rules:
EOL
cat >"${JOBSET_DEPLOY_DIR}/08_operand_role.yaml" <<EOL
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: jobset-operand
  namespace: openshift-jobset-operator
rules:
EOL

  for clusterRole in rbac.authorization.k8s.io_v1_clusterrole_*.yaml; do
    yq -oyaml ".rules" "$clusterRole" >> "${JOBSET_DEPLOY_DIR}/06_operand_clusterrole.yaml"
  done
  for role in rbac.authorization.k8s.io_v1_role_*.yaml; do
    yq -oyaml ".rules" "$role" >> "${JOBSET_DEPLOY_DIR}/08_operand_role.yaml"
  done
popd
