# Contributing to JobSet Operator

This document serves as a guide for contributing to the JobSet Operator, which is maintained by the OpenShift Control Plane group.

This document is explicitly for contributions to the JobSet Operator repository and not for high-level feature proposals within OpenShift.

Feature proposals should follow the OpenShift Enhancement Proposal process outlined in https://github.com/openshift/enhancements/blob/master/dev-guide/feature-zero-to-hero.md#openshift-feature-development-zero-to-hero-guide.
If you are looking for a review on an OpenShift Enhancement Proposal that involves changes to the JobSet Operator, please request a review in the [`#forum-ocp-apiserver`](https://redhat.enterprise.slack.com/archives/CB48XQ4KZ) Slack channel.

## Table of Contents

- [Code Conventions](#code-conventions)
- [Testing Guidelines](#testing-guidelines)
- [Pull Request Process and Guidelines](#pull-request-process-and-guidelines)
- [Review Expectations](#review-expectations)
- [Local Development Guide](#local-development-guide)

---

## Code Conventions

We largely follow the [Kubernetes Code Conventions](https://github.com/kubernetes/community/blob/main/contributors/guide/coding-conventions.md#code-conventions).

Review both the Kubernetes Code Conventions and the ones specified here. If any conventions are at odds with one another, prefer the conventions explicitly documented here.

### Bash

- Follow the [shell styleguide](https://google.github.io/styleguide/shellguide.html).
- Use [`shellcheck`](https://github.com/koalaman/shellcheck) to identify common mistakes or caveats.
- Ensure that all scripts run consistently across Linux and MacOS.

### Golang (Go)

- Review [Effective Go](https://go.dev/doc/effective_go).
- Review common [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
- Review and avoid [Go Landmines](https://gist.github.com/lavalamp/4bd23295a9f32706a48f)
- Comment your code following the [Go comment conventions](https://go.dev/doc/comment).
    - Comments should be meaningful and add context and/or explain choices that cannot be expressed through clear code.
    - All exported types, functions, and methods must have descriptive comments.
    - All unexported types, functions, and methods should have descriptive comments.
- When adding command-line flags, use dashes/hyphens (`-`) and not underscores (`_`).
- Naming
    - Please consider package name when selecting an interface name, and avoid redundancy. For example, `storage.Interface` is better than `storage.StorageInterface`.
    - Do not use uppercase characters, underscores, or dashes in package names.
    - Please consider parent directory name when choosing a package name. For example, `pkg/controllers/autoscaler/foo.go` should say `package autoscaler` not `package autoscalercontroller`.
        - Unless there's a good reason, the package foo line should match the name of the directory in which the .go file exists.
        - Importers can use a different name if they need to disambiguate.
    - Locks should be called `lock` and should never be embedded (always `lock sync.Mutex`). When multiple locks are present, give each lock a distinct name following Go conventions: `stateLock`, `mapLock` etc.
- Error handling
    - Wrap errors with meaningful context before returning or logging them.
- When logging, follow the [Kubernetes Logging Conventions](https://github.com/kubernetes/community/blob/main/contributors/devel/sig-instrumentation/logging.md).
- When patching OpenShift-maintained forks of "upstream" repositories, patches should be as small as reasonably possible and should minimize touch points with code that is likely to change and impact the rebasing process.

### General

Regardless of the programming language, make sure to take the following into consideration:
- Keep readability / maintainability in mind when writing code.
    - Clever code and abstractions are often harder to reason about after the fact. Keep clever code and abstractions to the minimum necessary to accomplish the end-goal.
- Do not reinvent the wheel. Where possible, use existing standard library or vendored library functionality. If you are adding a net-new dependency, stop and think if you _really_ need to add the new dependency to achieve your goals.
- When writing tests, focus on testing the functional behaviors your code exercises. Avoid writing tests that are testing that the standard library works as expected or is trivial. Do not write tests just for line coverage.

### Directory and File Conventions

- Avoid package sprawl. Find an appropriate subdirectory for new packages.
    - Libraries with no appropriate home belong in new package subdirectories of `pkg/util`.
- Avoid general utility packages. Packages called "util" are suspect. Instead, derive a name that describes your desired function. For example, the utility functions dealing with waiting for operations are in the `wait` package and include functionality like `Poll`. The full name is `wait.Poll`.
- All filenames should be lowercase.
- Go source files and directories use underscores, not dashes.
    - Package directories should generally avoid using separators as much as possible. When package names are multiple words, they usually should be in nested subdirectories.

---

## Testing Guidelines

These are high-level testing guidelines. The JobSet Operator has additional testing specifics documented in the [Testing Your Changes](#testing-your-changes) section below.

- All changes must include unit test additions/changes.
    - Exceptions are at reviewer/approver discretion.
- Table-driven unit tests are preferred for testing multiple scenarios/inputs. For an example, see https://github.com/openshift/cluster-authentication-operator/blob/a493799952e9b6838021ccc7d15d3d37d7ad3508/pkg/controllers/externaloidc/externaloidc_controller_test.go#L108 .
- Unit tests must pass on all platforms (at the very least, Linux + MacOS).
- Significant features should come with integration and/or end-to-end (e2e) tests where appropriate.
    - End-to-end tests _may_ be scoped as a separate work item when the end-to-end tests for the component must be added to the openshift/origin repository instead of the component repository. Adding e2e tests to the component repository is preferred where possible. It is up to reviewer/approver discretion whether a contribution can be merged without end-to-end tests being implemented.
- Do not expect an asynchronous thing to happen immediately. Do not wait for one second and expect a pod to be running. Wait and retry instead.

If necessary, manual integration testing can be done by creating a cluster using the [`Cluster Bot` Slack App](https://redhat.enterprise.slack.com/archives/D03KX7M1CRJ).
Once you have a cluster created, you can follow some of the instructions in https://github.com/openshift/enhancements/blob/master/dev-guide/operators.md for guidance on how to build component images and modify cluster-operators to deploy those images.

Most component repos have existing tooling to run unit tests. For JobSet Operator:

```bash
# Run unit tests
make test-unit

# Run e2e tests
export OPERATOR_IMAGE=quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}
export RELATED_IMAGE_OPERAND_IMAGE=quay.io/${QUAY_USER}/jobset:${IMAGE_TAG}
make test-e2e
```

---

## Pull Request Process and Guidelines

This section assumes that you have a functional understanding of `git` and how to create a pull request on GitHub.

If you do not, start with [GitHub's "Getting Started" guide](https://docs.github.com/en/get-started/start-your-journey).

### Prerequisites

Before you commit any changes or create any pull requests, you must adhere to OpenShift contribution policies.
Currently, that means enabling commit signature verification.

See https://docs.google.com/document/d/1184EPSGunUkcSQYUK8T4a6iyawwi6f2zxdbB2jtG9nQ/edit?usp=sharing for more details on how to adhere to the commit signature verification policy of OpenShift.

**To enable commit signing**:

```bash
git config --global user.signingkey <your-gpg-key-id>
git config --global commit.gpgsign true
```

### Creating a Pull Request

When creating a pull request, include the following:

- A brief, but descriptive, title.
    - All pull requests _should_ link to a Jira ticket associated with the work. There is automation that performs this linking when prefixing the title with the Jira ticket identifier like: `CNTRLPLANE-XXXX: my pull request title`. For pull requests that have no Jira ticket associated with it, you can prefix it with `NO-JIRA:` to signal that there is not a Jira ticket associated with it.
- A useful description of the changes being made and why they are important. Include links to supporting documents and any additional context that reviewers may need.

**Example PR Title**:
```
CNTRLPLANE-1234: add support for custom operand image override
```

or

```
NO-JIRA: fix typo in CONTRIBUTING.md
```

### CI / CD

For CI/CD, OpenShift uses Prow to run various checks. This can include unit tests, e2e tests, linters, etc.

The jobs configured for each repository are in https://github.com/openshift/release/tree/main/ci-operator/config/openshift . If you find yourself needing to add additional jobs, review the documentation at https://docs.ci.openshift.org/how-tos/contributing-openshift-release/ .

There are often a mixture of required and optional checks as well as merge criteria that must be met before a pull request can merge.
When any of these checks fail, the GitHub Prow bot will leave a comment on the PR with links to the run of that check that failed.

As the PR author, it is your responsibility to evaluate the failed checks and determine if there are any changes necessary to pass the checks.
If you suspect that the check failure was a flake, you can trigger retests by commenting `/retest` (or `/retest-required` for retesting only the required checks) on the PR.

**Common Prow Commands**:
- `/retest` - Rerun all failed tests
- `/retest-required` - Rerun only required failed tests
- `/test <job-name>` - Run a specific test job

### Verifying your changes / Creating an OpenShift cluster from a PR

As part of merging a PR, there is a requirement to verify that the changes you've made are working as expected using the `/verified` comment command.

While there are a lot of scenarios where the existing CI/CD checks may be sufficient to verify your changes are working (and can be denoted by commenting `/verified by ci`),
there may be scenarios where manual verification is required.

You can use the `Cluster Bot` Slack App to create a cluster from a PR by sending it a message in the format of `launch ${OCP_VERSION},${PR_LINK} ${PLATFORM},${VARIANT}`.
As an example, `launch 4.23,https://github.com/openshift/jobset-operator/pull/123 aws,techpreview` would launch an OpenShift 4.23 cluster with the changes made in openshift/jobset-operator#123 running on AWS with the TechPreviewNoUpgrade feature-set enabled.
For more information on what `Cluster Bot` can do, you can send it a message saying `help` and it will respond with additional documentation on how it can be used.

Once you've verified your changes work as expected, you can mark the PR as verified by commenting `/verified by @{your_github_handle}` on the PR.

### Additional Resources

For more information regarding more general OpenShift pull request processes, the following resources are helpful:

- https://docs.ci.openshift.org/architecture/jira
- https://docs.ci.openshift.org/
- https://steps.ci.openshift.org/

---

## Review Expectations

### Requesting a review

If you are not a member of the OpenShift control plane team and you need a review on a PR, post it in the [#forum-ocp-apiserver](https://redhat.enterprise.slack.com/archives/CB48XQ4KZ) Slack channel or reach out to folks outlined in the OWNERS file directly.

If you are a member of the OpenShift control plane team, reviews should come from your feature team. In the event your feature team does not have someone that can approve a PR, post it in the [#control-plane](https://redhat.enterprise.slack.com/archives/CC3CZCQHM) Slack channel.

OpenShift uses AI code review tools as part of the code review process.
Before requesting a review, address all feedback from the code review agent(s).
It is up to your discretion as the contributor how you would like to address that feedback.
Responding with an explanation as to why you are not going to take action on a comment made by the agent is an acceptable way to "address" its feedback.

### Interacting with reviewers

When interacting with reviewers/approvers:

- Be professional.
- Be respectful of differing opinions, viewpoints, and experiences.
- Gracefully give and receive constructive feedback.
- Focus on what is best for the product/organization, not just us as individuals.

A special note on the usage of AI - to respect the time of those that are reviewing your contribution, please do not use AI to respond to review comments.

---

## Local Development Guide

This section provides detailed guidance for local development, building, deploying, and debugging the JobSet Operator.

### Prerequisites

#### Required Tools

- **Go 1.25+**: [Installation guide](https://go.dev/doc/install)
- **Podman or Docker**: For building container images
- **oc CLI**: OpenShift command-line tool
- **Access to an OpenShift cluster**: 
  - CRC (CodeReady Containers) for local development
  - OpenShift cluster with cluster-admin access
  - Must have cert-manager installed (prerequisite)
- **Git**: For version control with commit signing enabled
- **Make**: For build automation

#### Optional Tools

- **golangci-lint**: For linting (auto-installed by `make lint`)
- **Ginkgo**: For running e2e tests (auto-installed by `make test-e2e`)

#### Verify Prerequisites

```bash
# Check Go version
go version  # Should be 1.25 or later

# Check cluster access
oc version
oc whoami

# Verify cert-manager is installed
oc get crd certificates.cert-manager.io

# Verify commit signing is enabled
git config --get user.signingkey
git config --get commit.gpgsign  # Should return 'true'
```

If cert-manager is not installed, install it:
```bash
oc apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.0/cert-manager.yaml
```

### Getting Started

#### Fork and Clone

1. **Fork the repository** on GitHub

2. **Clone your fork**:
   ```bash
   git clone https://github.com/<your-username>/jobset-operator.git
   cd jobset-operator
   ```

3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/openshift/jobset-operator.git
   git fetch upstream
   ```

#### Understand the Codebase

Review the documentation:
- [AGENTS.md](./AGENTS.md) - Code patterns and conventions for AI agents
- [ARCHITECTURE.md](./ARCHITECTURE.md) - System design and architecture
- [README.md](./README.md) - Quick overview

Key directories:
```
jobset-operator/
├── cmd/jobset-operator/     # Main entrypoint
├── pkg/operator/            # Core operator logic
├── pkg/apis/                # API definitions
├── deploy/                  # Deployment manifests
├── bindata/                 # Embedded assets
├── test/e2e/                # End-to-end tests
└── hack/                    # Build and generation scripts
```

### Local Development Setup

#### Step 1: Set Up Your Container Registry

You'll need a container registry to push images. We recommend [quay.io](https://quay.io).

1. **Create a Quay account** if you don't have one
2. **Create a public repository**: `<your-username>/jobset-operator`
3. **Create a public repository**: `<your-username>/jobset` (for operand)

#### Step 2: Log in to Your Registry

```bash
export QUAY_USER=<your-quay-username>
podman login quay.io -u ${QUAY_USER}
```

#### Step 3: Build and Push the Operator Image

```bash
export IMAGE_TAG=dev-$(git rev-parse --short HEAD)

# Build the operator image
podman build -f Dockerfile.ci -t quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG} .

# Push to registry
podman push quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}
```

**Note**: Make sure your Quay repository is **public** so the cluster can pull it.

#### Step 4: Build and Push the Operand Image

The operand is the JobSet controller from the OpenShift fork.

```bash
# Clone the operand repo
mkdir -p $HOME/go/src/sigs.k8s.io
cd $HOME/go/src/sigs.k8s.io
git clone https://github.com/openshift/kubernetes-sigs-jobset
cd kubernetes-sigs-jobset

# Check out the version from operand-git-ref
OPERAND_VERSION=$(cat <path-to-jobset-operator>/operand-git-ref)
git checkout ${OPERAND_VERSION}

# Build and push
podman build -t quay.io/${QUAY_USER}/jobset:${IMAGE_TAG} .
podman push quay.io/${QUAY_USER}/jobset:${IMAGE_TAG}

# Return to operator directory
cd <path-to-jobset-operator>
```

#### Step 5: Update Deployment Manifests

Update the operator deployment to use your images:

```bash
# Update operator image
sed -i "s|image: .*jobset-operator.*|image: quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}|g" deploy/12_deployment.yaml

# Update operand image
sed -i "s|value: .*jobset:.*|value: quay.io/${QUAY_USER}/jobset:${IMAGE_TAG}|g" deploy/12_deployment.yaml
```

#### Step 6: Deploy to Your Cluster

```bash
# Ensure you're logged into your cluster
oc login <your-cluster-url>

# Apply all deployment manifests
oc apply -f deploy/ --server-side
```

This creates:
- Namespace: `openshift-jobset-operator`
- CRD: `jobsetoperators.operator.openshift.io`
- RBAC: ServiceAccount, ClusterRole, ClusterRoleBinding, Role, RoleBinding
- Operator Deployment
- JobSetOperator CR named `cluster`

#### Step 7: Verify the Deployment

```bash
# Check operator pod is running
oc get pods -n openshift-jobset-operator

# Check operator logs
oc logs -n openshift-jobset-operator -l app=jobset-operator -f

# Check JobSetOperator status
oc get jobsetoperator cluster -o yaml

# Check conditions
oc get jobsetoperator cluster -o jsonpath='{.status.conditions}' | jq
```

Expected conditions:
- `Available: True`
- `Degraded: False`
- `Progressing: False`

### Building and Deploying

#### Quick Rebuild and Redeploy

After making code changes:

```bash
# 1. Rebuild and push operator image
export IMAGE_TAG=dev-$(date +%s)
podman build -f Dockerfile.ci -t quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG} .
podman push quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}

# 2. Update deployment
sed -i "s|image: quay.io/${QUAY_USER}/jobset-operator:.*|image: quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}|g" deploy/12_deployment.yaml

# 3. Re-apply deployment
oc apply -f deploy/12_deployment.yaml

# 4. Watch rollout
oc rollout status deployment/jobset-operator -n openshift-jobset-operator

# 5. Check logs
oc logs -n openshift-jobset-operator -l app=jobset-operator -f
```

#### Building with Make

The project uses OpenShift's build-machinery-go:

```bash
# Build binary locally
make build

# Run unit tests
make test-unit

# Run linting
make lint

# Run all verification
make verify
```

### Testing Your Changes

#### Unit Tests

```bash
# Run unit tests (pkg/ and cmd/)
make test-unit

# Run with coverage
go test -coverprofile=coverage.out ./pkg/... ./cmd/...
go tool cover -html=coverage.out
```

**Note**: This operator relies heavily on e2e tests; unit test coverage is minimal.

#### End-to-End Tests

##### Operator E2E Tests

These tests verify the operator's behavior:

```bash
# Set required environment variables
export OPERATOR_IMAGE=quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}
export RELATED_IMAGE_OPERAND_IMAGE=quay.io/${QUAY_USER}/jobset:${IMAGE_TAG}

# Optional: Set artifact directory for test results
export ARTIFACT_DIR=/tmp/jobset-operator-e2e

# Run e2e tests
make test-e2e
```

**What the tests verify**:
1. Operator deploys successfully
2. Operand (JobSet controller) starts and becomes available
3. Conditions are set correctly (Available, Degraded)
4. Pod recovery works (operator recreates deleted operand pods)
5. Management state transitions work (Unmanaged, Removed)

##### Operand E2E Tests

These tests verify the JobSet controller functionality:

```bash
# Run operand e2e tests (from upstream)
make test-e2e-operand
```

#### Manual Testing Examples

##### Test 1: Create a Simple JobSet

```bash
# Create a test namespace
oc create namespace jobset-test

# Create a simple JobSet
cat <<EOF | oc apply -f -
apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
metadata:
  name: simple-jobset
  namespace: jobset-test
spec:
  replicatedJobs:
  - name: workers
    replicas: 2
    template:
      spec:
        parallelism: 1
        completions: 1
        template:
          spec:
            containers:
            - name: worker
              image: busybox:latest
              command: ["sh", "-c", "echo 'Hello from worker'; sleep 10"]
            restartPolicy: Never
EOF

# Watch JobSet status
oc get jobset -n jobset-test -w

# Cleanup
oc delete jobset simple-jobset -n jobset-test
oc delete namespace jobset-test
```

### Code Generation

#### When to Regenerate

Run `make generate` after changing:

1. **API Types**: `pkg/apis/openshiftoperator/v1/types_jobsetoperator.go`
   - Regenerates: CRD, deepcopy, defaults, clients, informers

2. **Operand Version**: `operand-git-ref`
   - Regenerates: Upstream manifests in `bindata/assets/jobset-controller-generated/`

3. **Adding New Resources**: New files in `bindata/assets/`
   - Regenerates: `bindata/assets.go`

#### Generation Scripts

```bash
# Individual generators
make generate-clients              # Clientset/informers/listers
make regen-crd                     # CRD from API types
make generate-controller-manifests # Upstream operand manifests
make update-cluster-service-version # OLM bundle

# All generation
make generate

# Verification (run before committing)
make verify-codegen
make verify-controller-manifests
```

### Debugging

#### View Logs

```bash
# Operator logs
oc logs -n openshift-jobset-operator -l app=jobset-operator -f

# Operand logs
oc logs -n openshift-jobset-operator -l control-plane=controller-manager -f

# Previous container logs (if pod crashed)
oc logs -n openshift-jobset-operator -l app=jobset-operator -p
```

#### Enable Debug Logging

```bash
# Set operator log level to Debug
oc patch jobsetoperator cluster --type=merge -p '{"spec":{"logLevel":"Debug"}}'

# For even more verbose logging (Trace)
oc patch jobsetoperator cluster --type=merge -p '{"spec":{"logLevel":"Trace"}}'
```

#### Check Events

```bash
# Operator events
oc get events -n openshift-jobset-operator --sort-by='.lastTimestamp'

# JobSetOperator events
oc describe jobsetoperator cluster
```

#### Common Issues

**Issue**: Operator pod is CrashLooping
```bash
# Check logs
oc logs -n openshift-jobset-operator -l app=jobset-operator --tail=100

# Common causes:
# 1. OPERAND_IMAGE env var not set
oc get deployment jobset-operator -n openshift-jobset-operator -o yaml | grep OPERAND_IMAGE

# 2. RBAC issues
oc get clusterrolebinding | grep jobset
oc get rolebinding -n openshift-jobset-operator
```

**Issue**: Operand not starting (Degraded condition)
```bash
# Check status message
oc get jobsetoperator cluster -o jsonpath='{.status.conditions[?(@.type=="Degraded")]}' | jq

# Common causes:
# 1. cert-manager not installed
oc get crd certificates.cert-manager.io

# 2. Certificates not ready
oc get certificate -n openshift-jobset-operator
oc describe certificate jobset-serving-cert -n openshift-jobset-operator

# 3. Image pull errors
oc describe pod -n openshift-jobset-operator -l control-plane=controller-manager
```

---

## Additional Resources

- [OpenShift Operator Best Practices](https://docs.openshift.com/container-platform/latest/operators/operator_sdk/osdk-about.html)
- [library-go Documentation](https://github.com/openshift/library-go)
- [Kubernetes Controller Best Practices](https://kubernetes.io/docs/concepts/architecture/controller/)
- [JobSet Documentation](https://jobset.sigs.k8s.io/)
- [OpenShift CI Documentation](https://docs.ci.openshift.org/)

---

**Questions?** 

- Open an issue on GitHub
- Review the OWNERS file for maintainer contacts

**Thank you for contributing!**
