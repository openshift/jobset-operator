FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.25 as builder
WORKDIR /go/src/github.com/openshift/jobset-operator
COPY . .

ARG OPERAND_IMAGE=registry.redhat.io/job-set/jobset-rhel9@sha256:340533812a35057134bf2b746300999677fd6beb12627cf49f343d9ce5e6ff4e
ARG REPLACED_OPERAND_IMG=\${OPERAND_IMAGE}

# Replace the operand image in deploy/05_deployment.yaml with the one specified by the OPERAND_IMAGE build argument.
RUN hack/replace-image.sh deploy $REPLACED_OPERAND_IMG $OPERAND_IMAGE
RUN hack/replace-image.sh manifests $REPLACED_OPERAND_IMG $OPERAND_IMAGE

ARG OPERATOR_IMAGE=registry.redhat.io/job-set/jobset-rhel9-operator@sha256:162bff69c0f101df4b889587bb9df1e0b038d385358e382664ede0d69f542c5c
ARG REPLACED_OPERATOR_IMG=\${OPERATOR_IMAGE}

# Replace the operand image in deploy/05_deployment.yaml with the one specified by the OPERATOR_IMAGE build argument.
RUN hack/replace-image.sh deploy $REPLACED_OPERATOR_IMG $OPERATOR_IMAGE
RUN hack/replace-image.sh manifests $REPLACED_OPERATOR_IMG $OPERATOR_IMAGE

RUN mkdir licenses
COPY LICENSE licenses/.

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:bb08f2300cb8d12a7eb91dddf28ea63692b3ec99e7f0fa71a1b300f2756ea829

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
LABEL release="1.0.0"
LABEL version="1.0.0"
LABEL url="https://github.com/openshift/jobset-operator"
LABEL vendor="Red Hat, Inc."
LABEL summary="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit."
LABEL io.k8s.display-name="Job Set" \
      io.k8s.description="This is an operator to manage Job Set" \
      io.openshift.tags="openshift,jobset-operator" \
      com.redhat.delivery.appregistry=true \
      maintainer="AOS workloads team, <aos-workloads-staff@redhat.com>"
USER 1001
