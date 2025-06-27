FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.23 as builder
WORKDIR /go/src/github.com/openshift/jobset-operator
COPY . .

RUN make build --warn-undefined-variables

FROM registry.redhat.io/rhel9-4-els/rhel-minimal:9.4
COPY --from=builder /go/src/github.com/openshift/jobset-operator/jobset-operator /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/jobset-operator/LICENSE /licenses/.

LABEL com.redhat.component="Job Set Operator"
LABEL description="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit. It aims to offer a unified API for deploying HPC (e.g., MPI) and AI/ML training workloads (PyTorch, Jax, Tensorflow etc.) on Kubernetes."
LABEL name="jobset-operator"
LABEL summary="JobSet is a Kubernetes-native API for managing a group of k8s Jobs as a unit."
LABEL io.k8s.display-name="Job Set" \
      io.k8s.description="This is an operator to manage the Job Set" \
      io.openshift.tags="openshift,jobset-operator" \
      com.redhat.delivery.appregistry=true \
      maintainer="AOS workloads team, <aos-workloads@redhat.com>"
USER 1001
