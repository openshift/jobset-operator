package e2e

import (
	"os"

	o "github.com/onsi/gomega"

	operatorconfigclient "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned"
	jobsetoperatorv1clientset "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned/typed/openshiftoperator/v1"

	apiextclientv1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func getConfig() *rest.Config {
	kubeconfig := os.Getenv("KUBECONFIG")
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	o.Expect(err).NotTo(o.HaveOccurred())
	return config
}

func GetKubeClient() *k8sclient.Clientset {
	client, err := k8sclient.NewForConfig(getConfig())
	o.Expect(err).NotTo(o.HaveOccurred())
	return client
}

func GetApiExtensionClient() *apiextclientv1.Clientset {
	client, err := apiextclientv1.NewForConfig(getConfig())
	o.Expect(err).NotTo(o.HaveOccurred())
	return client
}

func GetDynamicClient() dynamic.Interface {
	client, err := dynamic.NewForConfig(getConfig())
	o.Expect(err).NotTo(o.HaveOccurred())
	return client
}

func GetJobSetOperatorClient() jobsetoperatorv1clientset.JobSetOperatorInterface {
	client, err := operatorconfigclient.NewForConfig(getConfig())
	o.Expect(err).NotTo(o.HaveOccurred())
	return client.OpenShiftOperatorV1().JobSetOperators()
}
