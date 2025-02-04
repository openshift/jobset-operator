package operator

import (
	"context"
	"os"
	"time"

	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/loglevel"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/staticresourcecontroller"
	"github.com/openshift/library-go/pkg/operator/v1helpers"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"github.com/openshift/jobset-operator/bindata"
	operatorconfigclient "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/jobset-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/jobset-operator/pkg/operator/operatorclient"
)

const (
	operatorNamespace = "openshift-jobset-operator"
)

func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
	namespace := cc.OperatorNamespace
	if namespace == "openshift-config-managed" {
		// we need to fall back to our default namespace rather than library-go's when running outside the cluster
		namespace = operatorNamespace
	}
	kubeClient, err := kubernetes.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}
	apiextensionsClient, err := apiextensionsclient.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}

	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
		"",
		namespace,
	)

	operatorConfigClient, err := operatorconfigclient.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}
	operatorConfigInformers := operatorclientinformers.NewSharedInformerFactory(operatorConfigClient, 10*time.Minute)
	jobSetOperatorClient := &operatorclient.JobSetOperatorClient{
		Ctx:               ctx,
		OperatorNamespace: namespace,
		SharedInformer:    operatorConfigInformers.OpenShiftOperator().V1().JobSetOperators().Informer(),
		Lister:            operatorConfigInformers.OpenShiftOperator().V1().JobSetOperators().Lister(),
		OperatorClient:    operatorConfigClient.OpenShiftOperatorV1(),
	}

	staticResourceController := staticresourcecontroller.NewStaticResourceController(
		"JobSetOperatorStaticResources",
		func(name string) ([]byte, error) {
			bytes, err := bindata.Asset(name)
			if err != nil {
				return nil, err
			}
			object, err := resourceread.ReadGenericWithUnstructured(bytes)
			if err != nil {
				return nil, err
			}

			jobSetOperatorObjectMeta, err := jobSetOperatorClient.GetObjectMeta()
			if err != nil {
				return nil, err
			}
			metadata, err := meta.Accessor(object)
			if err != nil {
				return nil, err
			}
			// set owner reference
			newOwnerRefs := append(metadata.GetOwnerReferences(), metav1.OwnerReference{
				APIVersion: "operator.openshift.io/v1",
				Kind:       "JobSetOperator",
				Name:       jobSetOperatorObjectMeta.Name,
				UID:        jobSetOperatorObjectMeta.UID,
			})
			metadata.SetOwnerReferences(newOwnerRefs)
			// set namespace
			if metadata.GetNamespace() != "" {
				metadata.SetNamespace(namespace)
			}
			switch t := object.(type) {
			case *v1.RoleBinding:
				for i := range t.Subjects {
					t.Subjects[i].Namespace = namespace
				}
			case *v1.ClusterRoleBinding:
				for i := range t.Subjects {
					t.Subjects[i].Namespace = namespace
				}
			}

			out, err := yaml.Marshal(object)
			if err != nil {
				return nil, err
			}
			return out, nil
		},
		[]string{
			"assets/jobset-controller-generated/v1_serviceaccount_jobset-controller-manager.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_clusterrolebinding_jobset-metrics-reader-rolebinding.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_clusterrolebinding_jobset-manager-rolebinding.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_clusterrolebinding_jobset-proxy-rolebinding.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_clusterrole_jobset-manager-role.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_clusterrole_jobset-metrics-reader.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_clusterrole_jobset-proxy-role.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_rolebinding_jobset-leader-election-rolebinding.yaml",
			"assets/jobset-controller-generated/rbac.authorization.k8s.io_v1_role_jobset-leader-election-role.yaml",
			"assets/jobset-controller-generated/v1_service_jobset-controller-manager-metrics-service.yaml",
		},
		(&resourceapply.ClientHolder{}).WithKubernetes(kubeClient),
		jobSetOperatorClient,
		cc.EventRecorder,
	).
		AddKubeInformers(kubeInformersForNamespaces)

	targetConfigReconciler := NewTargetConfigReconciler(
		os.Getenv("IMAGE"),
		namespace,
		operatorConfigInformers.OpenShiftOperator().V1().JobSetOperators(),
		kubeInformersForNamespaces,
		jobSetOperatorClient,
		kubeClient,
		dynamicClient,
		apiextensionsClient,
		discoveryClient,
		cc.EventRecorder,
	)

	logLevelController := loglevel.NewClusterOperatorLoggingController(jobSetOperatorClient, cc.EventRecorder)

	klog.Infof("Starting informers")
	operatorConfigInformers.Start(ctx.Done())
	kubeInformersForNamespaces.Start(ctx.Done())

	klog.Infof("Starting log level controller")
	go logLevelController.Run(ctx, 1)
	klog.Infof("Starting static resource controller")
	go staticResourceController.Run(ctx, 1)
	klog.Infof("Starting target config reconciler")
	go targetConfigReconciler.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
