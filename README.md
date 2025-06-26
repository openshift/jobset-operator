# JobSet Operator

The JobSet Operator provides the ability to deploy a
[JobSet controller](https://github.com/openshift/kubernetes-sigs-jobset) in OpenShift.

## Deploy the Operator

### Quick Development

1. Build and push the operator image to a registry:
   ```sh
   export QUAY_USER=${your_quay_user_id}
   export IMAGE_TAG=${your_image_tag}
   podman build -f Dockerfile.ci -t quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG} .
   podman login quay.io -u ${QUAY_USER}
   podman push quay.io/${QUAY_USER}/jobset-operator:${IMAGE_TAG}
   ```
2. Update the image spec under `.spec.template.spec.containers[0].image` field in the `deploy/12_deployment.yaml` Deployment to point to the newly built image
3. Build and push the operand image to a registry:
   ```sh
   mkdir -p $HOME/go/src/sigs.k8s.io
   cd $HOME/go/src/sigs.k8s.io
   git clone https://github.com/openshift/kubernetes-sigs-jobset
   cd $HOME/go/src/sigs.k8s.io/kubernetes-sigs-jobset
   export QUAY_USER=${your_quay_user_id}
   export IMAGE_TAG=${your_image_tag}
   podman build -t quay.io/${QUAY_USER}/jobset:${IMAGE_TAG} .
   podman login quay.io -u ${QUAY_USER}
   podman push quay.io/${QUAY_USER}/jobset:${IMAGE_TAG}
   ```
4. Update the image spec under `.spec.template.spec.containers[0].env[2].value` (`.name == "OPERAND_IMAGE"`) field in the `deploy/12_deployment.yaml` Deployment to point to the newly built image.
5. Make sure cert-manager is installed.
6. Apply the manifests from `deploy` directory:
   ```sh
   oc apply -f deploy/ --server-side
   ```

## E2E Test
Set kubeconfig to point to a OCP cluster

Set OPERATOR_IMAGE to point to your operator image

Set RELATED_IMAGE_OPERAND_IMAGE to point to your jobset image you want to test

[Optional] Set ARTIFACT_DIR to /path/to/dir for junit_report.xml

Run operator e2e test
```sh
make test-e2e
```
Run operand e2e test
```sh
make test-e2e-operand
```