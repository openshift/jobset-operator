package operator

import (
	"context"
	"fmt"
	"strconv"
	"time"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	"github.com/openshift/jobset-operator/bindata"
	jobsetoperatorsv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"
	operatorclientinformers "github.com/openshift/jobset-operator/pkg/generated/informers/externalversions/openshiftoperator/v1"
	openshiftoperatorv1 "github.com/openshift/jobset-operator/pkg/generated/listers/openshiftoperator/v1"
	"github.com/openshift/jobset-operator/pkg/operator/operatorclient"
)

type TargetConfigReconciler struct {
	targetImagePullSpec string
	operatorNamespace   string

	jobSetOperatorClient       *operatorclient.JobSetOperatorClient
	kubeClient                 kubernetes.Interface
	apiextensionsClient        *apiextensionsclient.Clientset
	eventRecorder              events.Recorder
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces
	jobSetOperatorsLister      openshiftoperatorv1.JobSetOperatorLister
	secretsLister              v1.SecretLister
	resourceCache              resourceapply.ResourceCache
}

func NewTargetConfigReconciler(targetImagePullSpec, operatorNamespace string,
	operatorClientInformer operatorclientinformers.JobSetOperatorInformer,
	kubeInformersForNamespaces v1helpers.KubeInformersForNamespaces,
	jobSetOperatorClient *operatorclient.JobSetOperatorClient,
	kubeClient kubernetes.Interface,
	apiextensionsClient *apiextensionsclient.Clientset,
	eventRecorder events.Recorder,
) factory.Controller {
	t := &TargetConfigReconciler{
		targetImagePullSpec:   targetImagePullSpec,
		operatorNamespace:     operatorNamespace,
		jobSetOperatorClient:  jobSetOperatorClient,
		kubeClient:            kubeClient,
		apiextensionsClient:   apiextensionsClient,
		secretsLister:         kubeInformersForNamespaces.SecretLister(),
		jobSetOperatorsLister: operatorClientInformer.Lister(),
		eventRecorder:         eventRecorder,
		resourceCache:         resourceapply.NewResourceCache(),
	}
	return factory.New().WithInformers(
		// for the operator changes
		operatorClientInformer.Informer(),
		// for the deployment and its configmap and secret
		kubeInformersForNamespaces.InformersFor(operatorNamespace).Apps().V1().Deployments().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorNamespace).Core().V1().ConfigMaps().Informer(),
		kubeInformersForNamespaces.InformersFor(operatorNamespace).Core().V1().Secrets().Informer(),
	).
		ResyncEvery(time.Minute*5).
		WithSyncDegradedOnError(jobSetOperatorClient).
		WithSync(t.sync).
		ToController("TargetConfigController", eventRecorder)
}

func (t TargetConfigReconciler) sync(ctx context.Context, syncCtx factory.SyncContext) error {
	jobSetOperator, err := t.jobSetOperatorsLister.JobSetOperators(t.operatorNamespace).Get(operatorclient.OperatorConfigName)
	if err != nil {
		klog.ErrorS(err, "unable to get operator configuration", "namespace", t.operatorNamespace, "jobset", operatorclient.OperatorConfigName)
		return err
	}

	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1",
		Kind:       "JobSetOperator",
		Name:       jobSetOperator.Name,
		UID:        jobSetOperator.UID,
	}

	specAnnotations := map[string]string{
		"jobsetoperators.operator.openshift.io/cluster": strconv.FormatInt(jobSetOperator.Generation, 10),
	}

	_, _, err = t.manageWebhookService(ctx, ownerReference)
	if err != nil {
		return err
	}
	webhookSecret, _, err := t.manageWebhookCertSecret()
	if err != nil {
		return err
	}
	specAnnotations["secrets/"+webhookSecret.Name] = webhookSecret.ResourceVersion

	_, _, err = t.manageValidatingWebhook(ctx, ownerReference)
	if err != nil {
		return err
	}
	_, _, err = t.manageMutatingWebhook(ctx, ownerReference)
	if err != nil {
		return err
	}
	_, _, err = t.manageCRD(ctx, ownerReference)
	if err != nil {
		return err
	}
	configMap, _, err := t.manageConfigMap(ctx, ownerReference)
	if err != nil {
		return err
	}
	specAnnotations["configmaps/"+configMap.Name] = configMap.ResourceVersion

	deployment, _, err := t.manageDeployment(ctx, jobSetOperator, ownerReference, specAnnotations, configMap)
	if err != nil {
		return err
	}

	_, _, err = v1helpers.UpdateStatus(ctx, t.jobSetOperatorClient, func(status *operatorv1.OperatorStatus) error {
		resourcemerge.SetDeploymentGeneration(&status.Generations, deployment)
		return nil
	})
	return err
}

func (t *TargetConfigReconciler) manageWebhookService(ctx context.Context, ownerReference metav1.OwnerReference) (*corev1.Service, bool, error) {
	webhookSecret := resourceread.ReadSecretV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/v1_secret_jobset-webhook-server-cert.yaml"))
	service := resourceread.ReadServiceV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/v1_service_jobset-webhook-service.yaml"))
	service.Namespace = t.operatorNamespace
	service.OwnerReferences = []metav1.OwnerReference{ownerReference}
	if service.Annotations == nil {
		service.Annotations = map[string]string{}
	}
	service.Annotations["service.beta.openshift.io/serving-cert-secret-name"] = webhookSecret.Name

	return resourceapply.ApplyService(ctx, t.kubeClient.CoreV1(), t.eventRecorder, service)
}

func (t *TargetConfigReconciler) manageWebhookCertSecret() (*corev1.Secret, bool, error) {
	secret := resourceread.ReadSecretV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/v1_secret_jobset-webhook-server-cert.yaml"))
	secret, err := t.secretsLister.Secrets(t.operatorNamespace).Get(secret.Name)
	// secret should be generated by the service-ca operator
	if err != nil {
		return nil, false, err
	}
	if len(secret.Data["tls.crt"]) == 0 || len(secret.Data["tls.key"]) == 0 {
		return nil, false, fmt.Errorf("%s secret is not initialized", secret.Name)
	}
	return secret, false, nil
}
func (t *TargetConfigReconciler) manageValidatingWebhook(ctx context.Context, ownerReference metav1.OwnerReference) (*admissionregistrationv1.ValidatingWebhookConfiguration, bool, error) {
	validatingWebhook := resourceread.ReadValidatingWebhookConfigurationV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/admissionregistration.k8s.io_v1_validatingwebhookconfiguration_jobset-validating-webhook-configuration.yaml"))
	for i := range validatingWebhook.Webhooks {
		if validatingWebhook.Webhooks[i].ClientConfig.Service != nil {
			validatingWebhook.Webhooks[i].ClientConfig.Service.Namespace = t.operatorNamespace
		}
	}
	validatingWebhook.OwnerReferences = []metav1.OwnerReference{ownerReference}
	if validatingWebhook.Annotations == nil {
		validatingWebhook.Annotations = map[string]string{}
	}
	validatingWebhook.Annotations["service.beta.openshift.io/inject-cabundle"] = "true"

	return resourceapply.ApplyValidatingWebhookConfigurationImproved(ctx, t.kubeClient.AdmissionregistrationV1(), t.eventRecorder, validatingWebhook, t.resourceCache)
}

func (t *TargetConfigReconciler) manageMutatingWebhook(ctx context.Context, ownerReference metav1.OwnerReference) (*admissionregistrationv1.MutatingWebhookConfiguration, bool, error) {
	mutatingWebhook := resourceread.ReadMutatingWebhookConfigurationV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/admissionregistration.k8s.io_v1_mutatingwebhookconfiguration_jobset-mutating-webhook-configuration.yaml"))
	for i := range mutatingWebhook.Webhooks {
		if mutatingWebhook.Webhooks[i].ClientConfig.Service != nil {
			mutatingWebhook.Webhooks[i].ClientConfig.Service.Namespace = t.operatorNamespace
		}
	}
	mutatingWebhook.OwnerReferences = []metav1.OwnerReference{ownerReference}
	if mutatingWebhook.Annotations == nil {
		mutatingWebhook.Annotations = map[string]string{}
	}
	mutatingWebhook.Annotations["service.beta.openshift.io/inject-cabundle"] = "true"

	return resourceapply.ApplyMutatingWebhookConfigurationImproved(ctx, t.kubeClient.AdmissionregistrationV1(), t.eventRecorder, mutatingWebhook, t.resourceCache)
}

func (t *TargetConfigReconciler) manageCRD(ctx context.Context, ownerReference metav1.OwnerReference) (*apiextensionsv1.CustomResourceDefinition, bool, error) {
	crd := resourceread.ReadCustomResourceDefinitionV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/apiextensions.k8s.io_v1_customresourcedefinition_jobsets.jobset.x-k8s.io.yaml"))
	if crd.Spec.Conversion != nil && crd.Spec.Conversion.Webhook != nil && crd.Spec.Conversion.Webhook.ClientConfig != nil && crd.Spec.Conversion.Webhook.ClientConfig.Service != nil {
		crd.Spec.Conversion.Webhook.ClientConfig.Service.Namespace = t.operatorNamespace
	}
	crd.OwnerReferences = []metav1.OwnerReference{ownerReference}
	if crd.Annotations == nil {
		crd.Annotations = map[string]string{}
	}
	crd.Annotations["service.beta.openshift.io/inject-cabundle"] = "true"

	currentCRD, err := t.apiextensionsClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crd.Name, metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		// no action needed
	case err != nil && !errors.IsNotFound(err):
		return nil, false, err
	case err == nil:
		if crd.Spec.Conversion != nil && crd.Spec.Conversion.Webhook != nil && crd.Spec.Conversion.Webhook.ClientConfig != nil {
			crd.Spec.Conversion.Webhook.ClientConfig.CABundle = currentCRD.Spec.Conversion.Webhook.ClientConfig.CABundle
		}
	}

	return resourceapply.ApplyCustomResourceDefinitionV1(ctx, t.apiextensionsClient.ApiextensionsV1(), t.eventRecorder, crd)
}

func (t *TargetConfigReconciler) manageConfigMap(ctx context.Context, ownerReference metav1.OwnerReference) (*corev1.ConfigMap, bool, error) {
	configData := bindata.MustAsset("assets/jobset-controller-config/config.yaml")
	configMap := resourceread.ReadConfigMapV1OrDie(bindata.MustAsset("assets/jobset-controller/config.yaml"))
	configMap.Namespace = t.operatorNamespace
	configMap.OwnerReferences = []metav1.OwnerReference{ownerReference}
	configMap.Data = map[string]string{
		"controller_manager_config.yaml": string(configData),
	}

	return resourceapply.ApplyConfigMap(ctx, t.kubeClient.CoreV1(), t.eventRecorder, configMap)
}

func (t *TargetConfigReconciler) manageDeployment(ctx context.Context, jobSetOperator *jobsetoperatorsv1.JobSetOperator, ownerReference metav1.OwnerReference, specAnnotations map[string]string, config *corev1.ConfigMap) (*appsv1.Deployment, bool, error) {
	required := resourceread.ReadDeploymentV1OrDie(bindata.MustAsset("assets/jobset-controller-generated/apps_v1_deployment_jobset-controller-manager.yaml"))
	required.Name = operatorclient.OperandName
	required.Namespace = t.operatorNamespace
	required.OwnerReferences = []metav1.OwnerReference{ownerReference}

	images := map[string]string{
		"${CONTROLLER_IMAGE}:latest": t.targetImagePullSpec,
	}
	for i := range required.Spec.Template.Spec.Containers {
		for pat, img := range images {
			if required.Spec.Template.Spec.Containers[i].Image == pat {
				required.Spec.Template.Spec.Containers[i].Image = img
				break
			}
		}
	}

	logLevel := ""
	switch jobSetOperator.Spec.LogLevel {
	case operatorv1.Normal:
		logLevel = "info"
	case operatorv1.Debug, operatorv1.Trace, operatorv1.TraceAll:
		logLevel = "debug"
	default:
		logLevel = "info"
	}
	newArgs := []string{
		"--config=/controller_manager_config.yaml",
		fmt.Sprintf("--zap-log-level=%s", logLevel),
	}
	// replace the default arg values from upstream
	required.Spec.Template.Spec.Containers[0].Args = newArgs

	resourcemerge.MergeMap(ptr.To(false), &required.Spec.Template.Annotations, specAnnotations)

	return resourceapply.ApplyDeployment(
		ctx,
		t.kubeClient.AppsV1(),
		t.eventRecorder,
		required,
		resourcemerge.ExpectedDeploymentGeneration(required, jobSetOperator.Status.Generations))
}
