# JobSet Operator

The JobSet Operator provides the ability to deploy a
[JobSet controller](https://github.com/kubernetes-sigs/jobset) in OpenShift.

## Deploy the Operator

### Quick Development

1. Build and push the operator image to a registry:
   ```sh
   export QUAY_USER=${your_quay_user_id}
   export IMAGE_TAG=${your_image_tag}
   podman build -t quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG} .
   podman login quay.io -u ${QUAY_USER}
   podman push quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}
   ```
2. Update the image spec under `.spec.template.spec.containers[0].image` field in the `deploy/12_deployment.yaml` Deployment to point to the newly built image
3. Build and push the operand image to a registry:
   ```sh
   mkdir -p $HOME/go/src/sigs.k8s.io
   cd $HOME/go/src/sigs.k8s.io
   git clone https://github.com/kubernetes-sigs/jobset
   cd $HOME/go/src/sigs.k8s.io/jobset
   export QUAY_USER=${your_quay_user_id}
   export IMAGE_TAG=${your_image_tag}
   podman build -t quay.io/${QUAY_USER}/jobset:${IMAGE_TAG} .
   podman login quay.io -u ${QUAY_USER}
   podman push quay.io/${QUAY_USER}/jobset:${IMAGE_TAG}
   ```
4. Update the image spec under `.spec.template.spec.containers[0].env[2].value` (`.name == "IMAGE"`) field in the `deploy/12_deployment.yaml` Deployment to point to the newly built image
5. Apply the manifests from `deploy` directory:
   ```sh
   oc apply -f deploy/
   ```
