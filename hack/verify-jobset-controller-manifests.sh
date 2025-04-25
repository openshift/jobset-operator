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
TMP_DIR=$(mktemp -d)

JOBSET_REPO_URL="https://github.com/openshift/kubernetes-sigs-jobset.git"
export JOBSET_BRANCH_OR_TAG="${JOBSET_BRANCH_OR_TAG:-$(cat "${SCRIPT_ROOT}/operand-git-ref")}"
export JOBSET_CONTROLLER_DIR="${JOBSET_CONTROLLER_DIR:-${TMP_DIR}/go/src/sigs.k8s.io/jobset}"

git clone --branch "$JOBSET_BRANCH_OR_TAG" "$JOBSET_REPO_URL" "$JOBSET_CONTROLLER_DIR"

"${SCRIPT_ROOT}/hack/update-jobset-controller-manifests.sh"

rm -rf "${TMP_DIR}"

pushd "${SCRIPT_ROOT}"

if [ -n "$(git status --porcelain -- bindata/assets/jobset-controller-generated/)" ];then
    popd
    echo "assets do not match with the github.com/openshift/kubernetes-sigs-jobset $JOBSET_BRANCH_OR_TAG. Please run update-jobset-controller-manifests.sh script" >&2
    exit 2
fi

if [ -n "$(git status --porcelain -- deploy/)" ];then
    popd
    echo "deploy assets do not match with the github.com/openshift/kubernetes-sigs-jobset $JOBSET_BRANCH_OR_TAG. Please run update-jobset-controller-manifests.sh script" >&2
    exit 2
fi

popd