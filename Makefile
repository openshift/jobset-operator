all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/images.mk \
	targets/openshift/deps.mk \
)

# Exclude e2e tests from unit testing
GO_TEST_PACKAGES :=./pkg/... ./cmd/...

IMAGE_REGISTRY :=registry.ci.openshift.org

# This will call a macro called "build-image" which will generate image specific targets based on the parameters:
# $0 - macro name
# $1 - target name
# $2 - image ref
# $3 - Dockerfile path
# $4 - context directory for image build
$(call build-image,ocp-jobset-operator,$(IMAGE_REGISTRY)/ocp/4.19:jobset-operator, ./Dockerfile,.)

$(call verify-golang-versions,Dockerfile)

test-e2e: GO_TEST_PACKAGES :=./test/e2e
# the e2e imports pkg/cmd which has a data race in the transport library with the library-go init code
test-e2e: GO_TEST_FLAGS :=-v
test-e2e: test-unit
.PHONY: test-e2e

regen-crd:
	go build -o _output/tools/bin/controller-gen ./vendor/sigs.k8s.io/controller-tools/cmd/controller-gen
	rm manifests/jobset-operator.crd.yaml
	./_output/tools/bin/controller-gen crd paths=./pkg/apis/openshiftoperator/v1/... output:crd:dir=./manifests
	mv manifests/operator.openshift.io_jobsetoperators.yaml manifests/jobset-operator.crd.yaml
	cp manifests/jobset-operator.crd.yaml deploy/00_jobset-operator.crd.yaml


generate-clients:
	GO=GO111MODULE=on GOFLAGS=-mod=readonly hack/update-codegen.sh
.PHONY: generate-clients

generate-controller-manifests:
	hack/update-jobset-controller-manifests.sh
.PHONY: generate-controller-manifests

generate: regen-crd generate-clients generate-controller-manifests
.PHONY: generate

verify-codegen:
	hack/verify-codegen.sh
.PHONY: verify-codegen

verify-controller-manifests:
	hack/verify-jobset-controller-manifests.sh
.PHONY: verify-controller-manifests

clean:
	$(RM) ./jobset-operator
	$(RM) -r ./_tmp
	$(RM) -r ./_output
.PHONY: clean
