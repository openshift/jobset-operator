FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.23-openshift-4.19 AS builder
WORKDIR /go/src/github.com/openshift/jobset-operator
COPY . .

RUN make build --warn-undefined-variables

FROM registry.ci.openshift.org/ocp/builder:rhel-9-base-openshift-4.19
COPY --from=builder /go/src/github.com/openshift/jobset-operator/jobset-operator /usr/bin/
# Upstream bundle and index images does not support versioning so
# we need to copy a specific version under /manifests layout directly
COPY --from=builder /go/src/github.com/openshift/jobset-operator/manifests/* /manifests/

LABEL io.k8s.display-name="OpenShift JobSet Operator" \
      io.k8s.description="This is a component of OpenShift and manages the JobSet controller" \
      io.openshift.tags="openshift,jobset-operator" \
      com.redhat.delivery.appregistry=true \
      maintainer="AOS workloads team, <aos-workloads@redhat.com>"
