FROM registry.redhat.io/openshift/golang-builder:golang-builder-v1.25-rhel9 as builder
WORKDIR /go/src/github.com/openshift/jobset-operator
COPY . .

ARG OPERAND_IMAGE=registry.redhat.io/job-set/jobset-rhel9@sha256:53c601a4edbd2408e1b3bc848c191eee1488144f34987322491c44c31b4fa524
ARG REPLACED_OPERAND_IMG=\${OPERAND_IMAGE}

# Replace the operand image in deploy/05_deployment.yaml with the one specified by the OPERAND_IMAGE build argument.
RUN hack/replace-image.sh deploy $REPLACED_OPERAND_IMG $OPERAND_IMAGE
RUN hack/replace-image.sh manifests $REPLACED_OPERAND_IMG $OPERAND_IMAGE

ARG OPERATOR_IMAGE=registry.redhat.io/job-set/jobset-rhel9-operator@sha256:fcd5050d7441ec04dfcb7b3cb17fdb8cf8c8896151feaf3162cdd235aebf8e4d
ARG REPLACED_OPERATOR_IMG=\${OPERATOR_IMAGE}

# Replace the operand image in deploy/05_deployment.yaml with the one specified by the OPERATOR_IMAGE build argument.
RUN hack/replace-image.sh deploy $REPLACED_OPERATOR_IMG $OPERATOR_IMAGE
RUN hack/replace-image.sh manifests $REPLACED_OPERATOR_IMG $OPERATOR_IMAGE

RUN mkdir licenses
COPY LICENSE licenses/.

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:1d7c5517a4a1a8e2688620b39ee980e82505ca1ab7ae5541b5463120ae9b3897

LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=job-set
LABEL operators.operatorframework.io.bundle.channels.v1=stable
LABEL operators.operatorframework.io.bundle.channel.default.v1=stable
LABEL operators.operatorframework.io.metrics.builder=operator-sdk-v1.34.2
LABEL operators.operatorframework.io.metrics.mediatype.v1=metrics+v1

COPY --from=builder /go/src/github.com/openshift/jobset-operator/manifests /manifests
COPY --from=builder /go/src/github.com/openshift/jobset-operator/metadata /metadata
COPY --from=builder /go/src/github.com/openshift/jobset-operator/licenses /licenses

LABEL com.redhat.component="Job Set Operator"
LABEL description="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit. It aims to offer a unified API for deploying HPC (e.g., MPI) and AI/ML training workloads (PyTorch, Jax, Tensorflow etc.) on Kubernetes."
LABEL distribution-scope="public"
LABEL name="job-set/jobset-operator-bundle"
LABEL cpe="cpe:/a:redhat:job_set:1.0::el9"
LABEL release="1.0.2"
LABEL version="1.0.2"
LABEL url="https://github.com/openshift/jobset-operator"
LABEL vendor="Red Hat, Inc."
LABEL summary="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit."
LABEL io.k8s.display-name="Job Set" \
      io.k8s.description="This is an operator to manage Job Set" \
      io.openshift.tags="openshift,jobset-operator" \
      com.redhat.delivery.appregistry=true \
      maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"
USER 1001
