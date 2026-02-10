FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.25 as builder
WORKDIR /go/src/github.com/openshift/jobset-operator
COPY . .

RUN make build --warn-undefined-variables

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest@sha256:759f5f42d9d6ce2a705e290b7fc549e2d2cd39312c4fa345f93c02e4abb8da95
COPY --from=builder /go/src/github.com/openshift/jobset-operator/jobset-operator /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/jobset-operator/LICENSE /licenses/.

LABEL com.redhat.component="Job Set Operator"
LABEL description="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit. It aims to offer a unified API for deploying HPC (e.g., MPI) and AI/ML training workloads (PyTorch, Jax, Tensorflow etc.) on Kubernetes."
LABEL name="job-set/jobset-rhel9-operator"
LABEL cpe="cpe:/a:redhat:job_set:1.0::el9"
LABEL summary="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit."
LABEL io.k8s.display-name="Job Set" \
      io.k8s.description="This is an operator to manage the Job Set" \
      io.openshift.tags="openshift,jobset-operator" \
      com.redhat.delivery.appregistry=true \
      maintainer="AOS workloads team, <aos-workloads@redhat.com>"
USER 1001
