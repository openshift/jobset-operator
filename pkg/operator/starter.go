package operator

import (
	"context"
	"time"

	"k8s.io/klog/v2"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/openshift/library-go/pkg/controller/controllercmd"
	"github.com/openshift/library-go/pkg/operator/loglevel"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	operatorconfigclient "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned"
	operatorclientinformers "github.com/openshift/jobset-operator/pkg/generated/informers/externalversions"
	"github.com/openshift/jobset-operator/pkg/operator/operatorclient"
)

func RunOperator(ctx context.Context, cc *controllercmd.ControllerContext) error {
	kubeClient, err := kubernetes.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(cc.ProtoKubeConfig)
	if err != nil {
		return err
	}

	kubeInformersForNamespaces := v1helpers.NewKubeInformersForNamespaces(kubeClient,
		"",
		operatorclient.OperatorNamespace,
	)

	operatorConfigClient, err := operatorconfigclient.NewForConfig(cc.KubeConfig)
	if err != nil {
		return err
	}
	operatorConfigInformers := operatorclientinformers.NewSharedInformerFactory(operatorConfigClient, 10*time.Minute)
	jobSetOperatorClient := &operatorclient.JobSetOperatorClient{
		Ctx:            ctx,
		SharedInformer: operatorConfigInformers.OpenShiftOperator().V1().JobSetOperators().Informer(),
		OperatorClient: operatorConfigClient.OpenShiftOperatorV1(),
	}

	_ = dynamicClient

	logLevelController := loglevel.NewClusterOperatorLoggingController(jobSetOperatorClient, cc.EventRecorder)

	klog.Infof("Starting informers")
	operatorConfigInformers.Start(ctx.Done())
	kubeInformersForNamespaces.Start(ctx.Done())

	klog.Infof("Starting log level controller")
	go logLevelController.Run(ctx, 1)

	<-ctx.Done()
	return nil
}
