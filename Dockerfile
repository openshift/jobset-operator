FROM brew.registry.redhat.io/rh-osbs/openshift-golang-builder:rhel_9_1.23 as builder
WORKDIR /go/src/github.com/openshift/jobset-operator
COPY . .

RUN make build --warn-undefined-variables

FROM registry.redhat.io/rhel9-4-els/rhel-minimal:9.4
COPY --from=builder /go/src/github.com/openshift/jobset-operator/jobset-operator /usr/bin/
RUN mkdir /licenses
COPY --from=builder /go/src/github.com/openshift/lws-operator/LICENSE /licenses/.

LABEL com.redhat.component="JobSet Operator"
LABEL description="JobSet Operator manages the JobSet."
LABEL name="jobset-operator"
LABEL summary="JobSet Operator manages the JobSet."
LABEL io.k8s.display-name="OpenShift JobSet Operator" \
      io.k8s.description="This is an operator to manage the JobSet" \
      io.openshift.tags="openshift,jobset-operator" \
      com.redhat.delivery.appregistry=true \
      maintainer="AOS workloads team, <aos-workloads@redhat.com>"
USER 1001
