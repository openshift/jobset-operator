all: build
.PHONY: all

# Use go.mod go version as a single source of truth of Ginkgo version. 
GINKGO_VERSION ?= $(shell go list -m -f '{{.Version}}' github.com/onsi/ginkgo/v2)

GOLANGCI_LINT = $(shell pwd)/_output/tools/bin/golangci-lint
GOLANGCI_LINT_VERSION ?= v2.1.6

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
$(call build-image,ocp-jobset-operator,$(IMAGE_REGISTRY)/ocp/4.20:jobset-operator, ./Dockerfile,.)

$(call verify-golang-versions,Dockerfile)

GINKGO = $(shell pwd)/_output/tools/bin/ginkgo
.PHONY: ginkgo
ginkgo: ## Download ginkgo locally if necessary.
	test -s $(shell pwd)/_output/tools/bin/ginkgo || GOFLAGS=-mod=readonly GOBIN=$(shell pwd)/_output/tools/bin go install github.com/onsi/ginkgo/v2/ginkgo@$(GINKGO_VERSION)

test-e2e: ginkgo
	RUN_OPERATOR_TEST=true GINKGO=$(GINKGO) hack/e2e-test.sh
.PHONY: test-e2e

test-e2e-operand: ginkgo
	RUN_OPERAND_TEST=true GINKGO=$(GINKGO) hack/e2e-test.sh
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

generate: generate-clients regen-crd generate-controller-manifests
.PHONY: generate

golangci-lint:
		@[ -f $(GOLANGCI_LINT) ] || { \
    	set -e ;\
    	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell dirname $(GOLANGCI_LINT)) $(GOLANGCI_LINT_VERSION) ;\
    	}
.PHONY: golangci-lint

lint: golangci-lint
	$(GOLANGCI_LINT) run --verbose --print-resources-usage
.PHONY: lint

verify-codegen:
	GO=GO111MODULE=on GOFLAGS=-mod=readonly hack/verify-codegen.sh
.PHONY: verify-codegen

verify-controller-manifests:
	hack/verify-jobset-controller-manifests.sh
.PHONY: verify-controller-manifests

clean:
	$(RM) ./jobset-operator
	$(RM) -r ./_tmp
	$(RM) -r ./_output
.PHONY: clean
