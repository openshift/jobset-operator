package e2e

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	v1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/jobset-operator/deploy"
	operatorv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"
	jobsetoperatorv1clientset "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned/typed/openshiftoperator/v1"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	networkingv1 "k8s.io/api/networking/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

const (
	oteOperatorNamespace = "openshift-jobset-operator"
	oteOperandLabel      = "control-plane=controller-manager"
	oteOperandName       = "jobset-controller-manager"
	oteNetworkPolicyName = "jobset-allow-operand"

	certManagerURL = "https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml"
)

//go:embed testdata/curl-test-pod.yaml
var curlPodTemplate string

//go:embed testdata/jobset-webhook-test.yaml
var jobsetWebhookTestYAML string

var deployTmpDir string

var _ = g.Describe("[sig-apps][Operator][Serial] JobSet Operator", g.Ordered, func() {
	var (
		ctx        context.Context
		cancelFnc  context.CancelFunc
		kubeClient *k8sclient.Clientset
	)

	g.BeforeAll(func() {
		var err error
		ctx, cancelFnc, kubeClient, err = setupOperator(g.GinkgoTB())
		o.Expect(err).NotTo(o.HaveOccurred())
	})

	g.AfterAll(func() {
		teardownOperator()
		cancelFnc()
	})

	g.It("should have correct operator conditions [Suite:openshift/jobset-operator/operator/serial]", func() {
		testOperatorConditions(g.GinkgoTB(), ctx, kubeClient)
	})

	g.It("should recover operand pods after deletion [Suite:openshift/jobset-operator/operator/serial]", func() {
		testOperandPodRecovery(g.GinkgoTB(), ctx, kubeClient)
	})

	g.It("should allow manual scaling when managementState is Unmanaged [Suite:openshift/jobset-operator/operator/serial]", func() {
		testUnmanagedScaling(g.GinkgoTB(), ctx, kubeClient)
	})

	g.It("should keep operand scaled when managementState is Removed [Suite:openshift/jobset-operator/operator/serial]", func() {
		testRemovedStateScaling(g.GinkgoTB(), ctx, kubeClient)
	})

	g.It("should create NetworkPolicy with correct spec [Suite:openshift/jobset-operator/operator/serial]", func() {
		expectNetworkPolicyValid(ctx, kubeClient, "NetworkPolicy should exist with correct spec")
	})

	g.It("should reconcile NetworkPolicy after mutation [Suite:openshift/jobset-operator/operator/serial]", func() {
		netpolClient := kubeClient.NetworkingV1().NetworkPolicies(oteOperatorNamespace)

		klog.Infof("Patching webhook port from 9443 to 1234")
		patch := []byte(`[{"op": "replace", "path": "/spec/ingress/0/ports/0/port", "value": 1234}]`)
		_, err := netpolClient.Patch(ctx, oteNetworkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to patch webhook port")

		expectNetworkPolicyValid(ctx, kubeClient, "operator should revert webhook port mutation")

		klog.Infof("Tampering monitoring namespace selector")
		patch = []byte(`[{"op": "replace", "path": "/spec/ingress/2/from/0/namespaceSelector/matchLabels", "value": {"kubernetes.io/metadata.name": "fake-namespace"}}]`)
		_, err = netpolClient.Patch(ctx, oteNetworkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to patch monitoring selector")

		expectNetworkPolicyValid(ctx, kubeClient, "operator should revert monitoring selector mutation")

		klog.Infof("Removing all egress rules")
		patch = []byte(`[{"op": "replace", "path": "/spec/egress", "value": []}]`)
		_, err = netpolClient.Patch(ctx, oteNetworkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to patch egress")

		expectNetworkPolicyValid(ctx, kubeClient, "operator should restore egress rules")
	})

	g.It("should recreate NetworkPolicy after deletion [Suite:openshift/jobset-operator/operator/serial]", func() {
		netpolClient := kubeClient.NetworkingV1().NetworkPolicies(oteOperatorNamespace)

		klog.Infof("Deleting NetworkPolicy %s", oteNetworkPolicyName)
		err := netpolClient.Delete(ctx, oteNetworkPolicyName, metav1.DeleteOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to delete NetworkPolicy")

		expectNetworkPolicyValid(ctx, kubeClient, "operator should recreate NetworkPolicy after deletion")
	})

	g.It("should recover NetworkPolicy after config drift on operator restart [Suite:openshift/jobset-operator/operator/serial]", func() {
		netpolClient := kubeClient.NetworkingV1().NetworkPolicies(oteOperatorNamespace)

		klog.Infof("Scaling down operator to 0 replicas")
		scaleDeployment(g.GinkgoTB(), ctx, kubeClient, "jobset-operator", 0)
		verifyPodCount(g.GinkgoTB(), ctx, kubeClient, oteOperatorNamespace, "name=jobset-operator", 0)

		klog.Infof("Wiping all ingress rules from NetworkPolicy")
		jsonPatch := []byte(`[{"op": "replace", "path": "/spec/ingress", "value": []}]`)
		_, err := netpolClient.Patch(ctx, oteNetworkPolicyName, types.JSONPatchType, jsonPatch, metav1.PatchOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to wipe ingress rules")

		netpol, err := netpolClient.Get(ctx, oteNetworkPolicyName, metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get NetworkPolicy after wipe")
		o.Expect(netpol.Spec.Ingress).To(o.BeEmpty(), "expected 0 ingress rules after wipe")

		klog.Infof("Scaling operator back up to 1 replica")
		scaleDeployment(g.GinkgoTB(), ctx, kubeClient, "jobset-operator", 1)

		o.Eventually(func() error {
			deploy, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Get(ctx, "jobset-operator", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if deploy.Status.ReadyReplicas < 1 {
				return fmt.Errorf("operator not ready yet: %d ready replicas", deploy.Status.ReadyReplicas)
			}
			return nil
		}, 2*time.Minute, 2*time.Second).Should(o.Succeed(), "operator should become ready")

		expectNetworkPolicyValid(ctx, kubeClient, "operator should recover NetworkPolicy after restart")
	})

	g.It("should allow webhook traffic on port 9443 [Suite:openshift/jobset-operator/operator/serial]", func() {
		operandPod, err := getOperandPod(ctx, kubeClient)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get operand pod")
		webhookURL := fmt.Sprintf("https://%s:9443", operandPod.Status.PodIP)

		code, err := runCurlPod(ctx, oteOperatorNamespace, webhookURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl from same namespace failed")
		o.Expect(code).NotTo(o.Equal("000"), "webhook port 9443 blocked from same namespace")

		code, err = runCurlPod(ctx, "default", webhookURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl from default namespace failed")
		o.Expect(code).NotTo(o.Equal("000"), "webhook port 9443 blocked from default namespace")
	})

	g.It("should allow webhook via kube-apiserver [Suite:openshift/jobset-operator/operator/serial]", func() {
		klog.Infof("Creating JobSet to test webhook via kube-apiserver")
		jobsetName, err := ocCreate(ctx, jobsetWebhookTestYAML)
		defer func() {
			_ = runCommand("oc", "delete", "jobset", jobsetName, "-n", "default", "--ignore-not-found")
		}()
		o.Expect(err).NotTo(o.HaveOccurred(), "webhook rejected JobSet creation")
		klog.Infof("JobSet %s created successfully", jobsetName)
	})

	g.It("should allow metrics from monitoring and block from random namespace [Suite:openshift/jobset-operator/operator/serial]", func() {
		operandPod, err := getOperandPod(ctx, kubeClient)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get operand pod")
		metricsURL := fmt.Sprintf("https://%s:8443", operandPod.Status.PodIP)

		code, err := runCurlPod(ctx, "openshift-monitoring", metricsURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl from openshift-monitoring failed")
		o.Expect(code).NotTo(o.Equal("000"), "metrics port 8443 blocked from openshift-monitoring")

		blockNS, err := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "test-netpol-e2e-"},
		}, metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create test namespace")
		defer func() {
			_ = kubeClient.CoreV1().Namespaces().Delete(ctx, blockNS.Name, metav1.DeleteOptions{})
		}()

		code, err = runCurlPod(ctx, blockNS.Name, metricsURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl from random namespace failed")
		o.Expect(code).To(o.Equal("000"), "metrics port 8443 should be blocked from random namespace")
	})

	g.It("should block traffic on unlisted port [Suite:openshift/jobset-operator/operator/serial]", func() {
		operandPod, err := getOperandPod(ctx, kubeClient)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get operand pod")
		blockedURL := fmt.Sprintf("https://%s:1234", operandPod.Status.PodIP)

		code, err := runCurlPod(ctx, "default", blockedURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl to blocked port failed")
		o.Expect(code).To(o.Equal("000"), "unlisted port 1234 should be blocked")
	})

	g.It("should allow egress from operand to API server [Suite:openshift/jobset-operator/operator/serial]", func() {
		code, err := runCommandInPod(ctx, oteOperatorNamespace, oteOperandName, "curl", "-w", "%{http_code}", "-s", "-o", "/dev/null", "https://kubernetes.default.svc:443/api")
		o.Expect(err).NotTo(o.HaveOccurred(), "curl to API server from operand failed")
		o.Expect(code).NotTo(o.Equal("000"), "API server access should be allowed for operand")
	g.It("should not reconcile when managementState is Unmanaged [Suite:openshift/jobset-operator/operator/serial]", func() {
		testUnmanagedState(g.GinkgoTB(), ctx, kubeClient)
	})

	g.It("should not reconcile when managementState is Removed [Suite:openshift/jobset-operator/operator/serial]", func() {
		testRemovedState(g.GinkgoTB(), ctx, kubeClient)
	})
})

func setupOperator(t testing.TB) (context.Context, context.CancelFunc, *k8sclient.Clientset, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	klog.Infof("Verifying required environment variables")
	if os.Getenv("KUBECONFIG") == "" {
		return nil, cancel, nil, fmt.Errorf("KUBECONFIG must be set")
	}

	if os.Getenv("OPERATOR_IMAGE") == "" && os.Getenv("RELATED_IMAGE_OPERAND_IMAGE") == "" {
		if os.Getenv("RELEASE_IMAGE_LATEST") == "" {
			return nil, cancel, nil, fmt.Errorf("RELEASE_IMAGE_LATEST must be set when OPERATOR_IMAGE and RELATED_IMAGE_OPERAND_IMAGE are not set")
		}
		if os.Getenv("NAMESPACE") == "" {
			return nil, cancel, nil, fmt.Errorf("NAMESPACE must be set when OPERATOR_IMAGE and RELATED_IMAGE_OPERAND_IMAGE are not set")
		}
	}

	var operatorImage string
	if os.Getenv("OPERATOR_IMAGE") != "" {
		operatorImage = os.Getenv("OPERATOR_IMAGE")
	} else {
		registry := strings.Split(os.Getenv("RELEASE_IMAGE_LATEST"), "/")[0]
		operatorImage = registry + "/" + os.Getenv("NAMESPACE") + "/pipeline:jobset-operator"
	}
	klog.Infof("Using operator image: %s", operatorImage)

	var operandImage string
	if os.Getenv("RELATED_IMAGE_OPERAND_IMAGE") != "" {
		operandImage = os.Getenv("RELATED_IMAGE_OPERAND_IMAGE")
	} else {
		registry := strings.Split(os.Getenv("RELEASE_IMAGE_LATEST"), "/")[0]
		operandImage = registry + "/" + os.Getenv("NAMESPACE") + "/pipeline:kubernetes-sigs-jobset"
	}
	klog.Infof("Using operand image: %s", operandImage)

	klog.Infof("Installing cert-manager")
	if err := runCommand("oc", "apply", "-f", certManagerURL); err != nil {
		return nil, cancel, nil, fmt.Errorf("failed to install cert-manager: %w", err)
	}
	if err := runCommand("oc", "-n", "cert-manager", "wait", "--for=condition=ready", "pod",
		"-l", "app.kubernetes.io/instance=cert-manager", "--timeout=2m"); err != nil {
		return nil, cancel, nil, fmt.Errorf("failed to wait for cert-manager: %w", err)
	}

	klog.Infof("Writing deploy manifests to temp directory")
	var err error
	deployTmpDir, err = os.MkdirTemp("", "jobset-deploy-")
	if err != nil {
		return nil, cancel, nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	entries, err := deploy.Assets.ReadDir(".")
	if err != nil {
		return nil, cancel, nil, fmt.Errorf("failed to read deploy assets: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := deploy.Assets.ReadFile(entry.Name())
		if err != nil {
			return nil, cancel, nil, fmt.Errorf("failed to read asset %s: %w", entry.Name(), err)
		}

		content := string(data)
		content = strings.ReplaceAll(content, "${OPERATOR_IMAGE}", operatorImage)
		content = strings.ReplaceAll(content, "${OPERAND_IMAGE}", operandImage)

		if err := os.WriteFile(filepath.Join(deployTmpDir, entry.Name()), []byte(content), 0644); err != nil {
			return nil, cancel, nil, fmt.Errorf("failed to write asset %s: %w", entry.Name(), err)
		}
	}

	klog.Infof("Applying deploy manifests")
	err = wait.PollUntilContextTimeout(ctx, 1*time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		if applyErr := runCommand("oc", "apply", "-f", deployTmpDir, "--server-side"); applyErr != nil {
			klog.Infof("oc apply failed (will retry): %v", applyErr)
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, cancel, nil, fmt.Errorf("failed to apply deploy manifests: %w", err)
	}

	klog.Infof("Waiting for operator deployment")
	if err := runCommand("oc", "wait", "deployment", "jobset-operator",
		"-n", oteOperatorNamespace, "--for=create", "--timeout=2m"); err != nil {
		return nil, cancel, nil, fmt.Errorf("failed waiting for operator deployment creation: %w", err)
	}
	if err := runCommand("oc", "wait", "deployment", "jobset-operator",
		"-n", oteOperatorNamespace, "--for=condition=Available", "--timeout=5m"); err != nil {
		return nil, cancel, nil, fmt.Errorf("failed waiting for operator deployment availability: %w", err)
	}

	klog.Infof("Waiting for operand deployment")
	if err := runCommand("oc", "wait", "deployment", oteOperandName,
		"-n", oteOperatorNamespace, "--for=create", "--timeout=2m"); err != nil {
		return nil, cancel, nil, fmt.Errorf("failed waiting for operand deployment creation: %w", err)
	}
	if err := runCommand("oc", "wait", "deployment", oteOperandName,
		"-n", oteOperatorNamespace, "--for=condition=Available", "--timeout=5m"); err != nil {
		return nil, cancel, nil, fmt.Errorf("failed waiting for operand deployment availability: %w", err)
	}

	klog.Infof("Operator and operand are ready")
	kubeClient := GetKubeClient()
	return ctx, cancel, kubeClient, nil
}

func teardownOperator() {
	if deployTmpDir != "" {
		_ = os.RemoveAll(deployTmpDir)
	}
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %v\n%s", name, args, err, string(out))
	}
	return nil
}

func testOperatorConditions(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	jobSetOperatorClient := GetJobSetOperatorClient()
	o.Eventually(func() error {
		jobsetOperators, err := jobSetOperatorClient.List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list JobSetOperators: %v", err)
		}
		if len(jobsetOperators.Items) != 1 {
			return fmt.Errorf("unexpected number of JobSetOperators %d", len(jobsetOperators.Items))
		}

		for _, condition := range jobsetOperators.Items[0].Status.Conditions {
			if strings.HasSuffix(condition.Type, v1.OperatorStatusTypeDegraded) && condition.Status == v1.ConditionTrue {
				return fmt.Errorf("degraded condition exists: %+v", jobsetOperators.Items[0].Status.Conditions)
			}
		}

		cond := v1helpers.FindOperatorCondition(jobsetOperators.Items[0].Status.Conditions, v1.OperatorStatusTypeAvailable)
		if cond == nil || cond.Status != v1.ConditionTrue {
			return fmt.Errorf("JobSet operator is not available")
		}
		return nil
	}, 5*time.Minute, 5*time.Second).Should(o.Succeed(), "operator should be available with no degraded conditions")
}

func testOperandPodRecovery(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	pods, err := kubeClient.CoreV1().Pods(oteOperatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: oteOperandLabel,
	})
	if err != nil {
		t.Fatalf("Failed to list operand pods: %v", err)
	}
	if len(pods.Items) == 0 {
		t.Fatalf("No operand pods found")
	}

	err = kubeClient.CoreV1().Pods(oteOperatorNamespace).DeleteCollection(
		ctx,
		metav1.DeleteOptions{
			GracePeriodSeconds: ptr.To[int64](30),
		},
		metav1.ListOptions{
			LabelSelector: oteOperandLabel,
		},
	)
	if err != nil {
		t.Fatalf("Failed to delete operand pods: %v", err)
	}

	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		newPods, err := kubeClient.CoreV1().Pods(oteOperatorNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: oteOperandLabel,
		})
		if err != nil {
			return false, err
		}

		activePods := make([]corev1.Pod, 0)
		for _, pod := range newPods.Items {
			if pod.DeletionTimestamp == nil {
				activePods = append(activePods, pod)
			}
		}
		if len(activePods) == 0 {
			return false, nil
		}
		for _, pod := range activePods {
			if pod.Status.Phase != corev1.PodRunning {
				klog.Infof("Pod %s status: %s", pod.Name, pod.Status.Phase)
				return false, nil
			}
			klog.Infof("Pod %s is Running", pod.Name)
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Failed waiting for operand pod recovery: %v", err)
	}
}

func testUnmanagedScaling(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	jobSetOperatorClient := GetJobSetOperatorClient()

	jobsetOperator, originalState, err := getOperatorState(ctx, jobSetOperatorClient)
	if err != nil {
		t.Fatalf("Failed to get operator state: %v", err)
	}
	originalPodCount := getPodCount(ctx, kubeClient, oteOperatorNamespace, oteOperandLabel)

	defer func() {
		setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, originalState)
		verifyPodCount(t, ctx, kubeClient, oteOperatorNamespace, oteOperandLabel, originalPodCount)
	}()

	setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, v1.Unmanaged)
	scaleDeployment(t, ctx, kubeClient, oteOperandName, 3)
	verifyPodCount(t, ctx, kubeClient, oteOperatorNamespace, oteOperandLabel, 3)
}

func testRemovedStateScaling(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	jobSetOperatorClient := GetJobSetOperatorClient()

	jobsetOperator, originalState, err := getOperatorState(ctx, jobSetOperatorClient)
	if err != nil {
		t.Fatalf("Failed to get operator state: %v", err)
	}
	originalPodCount := getPodCount(ctx, kubeClient, oteOperatorNamespace, oteOperandLabel)

	defer func() {
		newctx := context.TODO()
		setManagementState(t, newctx, jobSetOperatorClient, jobsetOperator, originalState)
		verifyPodCount(t, newctx, kubeClient, oteOperatorNamespace, oteOperandLabel, originalPodCount)
	}()

	setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, v1.Removed)
	scaleDeployment(t, ctx, kubeClient, oteOperandName, 3)
	verifyPodCount(t, ctx, kubeClient, oteOperatorNamespace, oteOperandLabel, 3)

	var genBaseline int64
	defer func() {
		newctx := context.TODO()
		setManagementState(t, newctx, jobSetOperatorClient, jobsetOperator, originalState)
		waitForOperatorAvailable(t, newctx, jobSetOperatorClient, genBaseline)
	}()

	setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, v1.Removed)
	// Give the operator a chance to finish an ongoing sync
	time.Sleep(1 * time.Second)
	genBaseline = operandDeploymentGeneration(jobsetOperator.Status.Generations)
	verifyOperatorDoesNotReconcile(t, ctx, kubeClient)
}

func getOperatorState(ctx context.Context, jobSetOperatorClient jobsetoperatorv1clientset.JobSetOperatorInterface) (*operatorv1.JobSetOperator, v1.ManagementState, error) {
	jobsetOperator, err := jobSetOperatorClient.Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get operator: %w", err)
	}
	return jobsetOperator, jobsetOperator.Spec.ManagementState, nil
}

func setManagementState(t testing.TB, ctx context.Context, jobSetOperatorClient jobsetoperatorv1clientset.JobSetOperatorInterface, operator *operatorv1.JobSetOperator, state v1.ManagementState) {
	t.Helper()
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current, getErr := jobSetOperatorClient.Get(ctx, operator.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		current.Spec.ManagementState = state
		_, updateErr := jobSetOperatorClient.Update(ctx, current, metav1.UpdateOptions{})
		return updateErr
	})
	if retryErr != nil {
		t.Fatalf("Failed to set management state to %s: %v", state, retryErr)
	}
}

func scaleDeployment(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, operandName string, replicas int32) {
	t.Helper()
	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Patch(
		ctx,
		operandName,
		types.StrategicMergePatchType,
		[]byte(patch),
		metav1.PatchOptions{})
	if err != nil {
		t.Fatalf("Failed to scale deployment %s to %d replicas: %v", operandName, replicas, err)
	}
}

func verifyPodCount(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, namespace, labelSelector string, expected int) {
	t.Helper()
	o.Eventually(func() int {
		return getPodCount(ctx, kubeClient, namespace, labelSelector)
	}, 5*time.Minute, 10*time.Second).Should(
		o.Equal(expected),
		"Pod count should reach %d", expected)
}

func getPodCount(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, labelSelector string) int {
	pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		klog.Errorf("Pod list error: %v\n", err)
		return -1
	}
	return len(pods.Items)
}

// expectNetworkPolicyValid waits for NetworkPolicy to exist and be valid
func expectNetworkPolicyValid(ctx context.Context, kubeClient *k8sclient.Clientset, msg string) {
	o.Eventually(func() error {
		netpol, err := kubeClient.NetworkingV1().NetworkPolicies(oteOperatorNamespace).Get(ctx, oteNetworkPolicyName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		return validateNetworkPolicySpec(netpol)
	}, 1*time.Minute, 2*time.Second).Should(o.Succeed(), msg)
}

// validateNetworkPolicySpec validates the complete NetworkPolicy specification
func validateNetworkPolicySpec(netpol *networkingv1.NetworkPolicy) error {
	// Validate PodSelector
	sel := netpol.Spec.PodSelector
	if sel.MatchLabels["app.kubernetes.io/name"] != "jobset" || sel.MatchLabels["control-plane"] != "controller-manager" {
		return fmt.Errorf("invalid podSelector labels: %v", sel.MatchLabels)
	}
	if len(sel.MatchExpressions) != 0 {
		return fmt.Errorf("podSelector should not have MatchExpressions, got %v", sel.MatchExpressions)
	}

	// Validate first ingress rule (webhook on port 9443)
	webhookRule := netpol.Spec.Ingress[0]
	if len(webhookRule.Ports) != 1 || webhookRule.Ports[0].Port.IntValue() != 9443 {
		return fmt.Errorf("webhook ingress rule: expected single port 9443, got %v", webhookRule.Ports)
	}

	// Validate last ingress rule (metrics from monitoring namespaces on port 8443)
	monitoringRule := netpol.Spec.Ingress[len(netpol.Spec.Ingress)-1]
	if len(monitoringRule.Ports) != 1 || monitoringRule.Ports[0].Port.IntValue() != 8443 {
		return fmt.Errorf("monitoring ingress rule: expected single port 8443, got %v", monitoringRule.Ports)
	}

	// Verify monitoring namespace selectors are present
	hasMonitoringSelector := false
	for _, from := range monitoringRule.From {
		if from.NamespaceSelector != nil {
			if from.NamespaceSelector.MatchLabels["openshift.io/cluster-monitoring"] == "true" {
				hasMonitoringSelector = true
				break
			}
		}
	}
	if !hasMonitoringSelector {
		return fmt.Errorf("monitoring rule: missing openshift.io/cluster-monitoring namespace selector")
	}

	// Validate Egress - must have unrestricted egress rule
	if len(netpol.Spec.Egress) == 0 {
		return fmt.Errorf("missing egress rules")
	}
	if len(netpol.Spec.Egress[0].Ports) != 0 || len(netpol.Spec.Egress[0].To) != 0 {
		return fmt.Errorf("expected unrestricted egress rule (empty Ports and To), got Ports=%v To=%v",
			netpol.Spec.Egress[0].Ports, netpol.Spec.Egress[0].To)
	}

	// Validate PolicyTypes - must include both Ingress and Egress
	policyTypes := fmt.Sprintf("%v", netpol.Spec.PolicyTypes)
	if !strings.Contains(policyTypes, "Ingress") || !strings.Contains(policyTypes, "Egress") {
		return fmt.Errorf("policyTypes must include both Ingress and Egress, got %v", netpol.Spec.PolicyTypes)
	}

	// Validate owner reference
	if netpol.OwnerReferences[0].Kind != "JobSetOperator" || netpol.OwnerReferences[0].Name != "cluster" {
		return fmt.Errorf("expected owner reference to JobSetOperator/cluster, got %s/%s",
			netpol.OwnerReferences[0].Kind, netpol.OwnerReferences[0].Name)
	}

	return nil
}

// scaleDeployment scales a deployment to the specified replica count
func scaleDeployment(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, name string, replicas int32) {
	t.Helper()
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
	_, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	o.ExpectWithOffset(1, err).NotTo(o.HaveOccurred(), "failed to scale deployment %s to %d replicas", name, replicas)
}

// verifyPodCount verifies that the expected number of pods exist with the given label selector
func verifyPodCount(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, namespace, labelSelector string, expectedCount int) {
	t.Helper()
	o.EventuallyWithOffset(1, func() int {
		pods, err := kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			return -1
		}
		count := 0
		for _, pod := range pods.Items {
			if pod.DeletionTimestamp == nil {
				count++
			}
		}
		return count
	}, 3*time.Minute, 2*time.Second).Should(o.Equal(expectedCount),
		"expected %d pods with label %s in namespace %s", expectedCount, labelSelector, namespace)
}

// getOperandPod returns a running operand pod with an assigned IP
func getOperandPod(ctx context.Context, kubeClient *k8sclient.Clientset) (*corev1.Pod, error) {
	pods, err := kubeClient.CoreV1().Pods(oteOperatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=jobset,control-plane=controller-manager",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list operand pods: %v", err)
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.DeletionTimestamp == nil && p.Status.Phase == corev1.PodRunning && p.Status.PodIP != "" {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no operand pod found in Running state with an assigned IP")
}

// runCurlPod creates a curl test pod and returns the HTTP status code from its logs
func runCurlPod(ctx context.Context, namespace, targetURL string) (string, error) {
	yaml := strings.ReplaceAll(curlPodTemplate, "{{NAMESPACE}}", namespace)
	yaml = strings.ReplaceAll(yaml, "{{TARGET_URL}}", targetURL)

	podName, err := ocCreate(ctx, yaml)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = runCommand("oc", "delete", "pod", podName, "-n", namespace, "--ignore-not-found")
	}()

	// Wait for pod completion (Succeeded or Failed — both are valid terminal states)
	_ = runCommand("oc", "wait", "pod", podName, "-n", namespace,
		"--for=jsonpath={.status.phase}=Succeeded", "--timeout=60s")
	_ = runCommand("oc", "wait", "pod", podName, "-n", namespace,
		"--for=jsonpath={.status.phase}=Failed", "--timeout=10s")

	cmd := exec.CommandContext(ctx, "oc", "logs", podName, "-n", namespace)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get curl pod logs: %v\n%s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// ocCreate writes yaml to a temp file, runs "oc create -f", and returns the created resource name
func ocCreate(ctx context.Context, yaml string) (string, error) {
	tmpFile, err := os.CreateTemp("", "e2e-*.yaml")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	if _, err := tmpFile.WriteString(yaml); err != nil {
		return "", err
	}
	if err := tmpFile.Close(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, "oc", "create", "-f", tmpFile.Name())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("oc create failed: %v\n%s", err, string(out))
	}
	name := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.TrimSuffix(name, " created"), nil
}

// runCommandInPod executes a command inside a pod and returns the output
func runCommandInPod(ctx context.Context, namespace, podName string, args ...string) (string, error) {
	kcmd := []string{"exec", podName, "-n", namespace, "--"}
	kcmd = append(kcmd, args...)
	cmd := exec.CommandContext(ctx, "oc", kcmd...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("oc exec failed: %v\n%s", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
// verifyOperatorDoesNotReconcile patches a template annotation on the operand
// deployment, which is a spec change that increments generation by 1 and
// triggers a rollout. It then waits for the rollout to complete
// (observedGeneration catches up) and verifies the operator did not apply
// another change on top (generation stays at genBefore+1, not genBefore+2).
func verifyOperatorDoesNotReconcile(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()

	dep, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Get(ctx, oteOperandName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get deployment: %v", err)
	}
	genBefore := dep.Generation

	patch := fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"test.openshift.io/rollout-trigger":"%s"}}}}}`,
		time.Now().Format(time.RFC3339Nano))
	_, err = kubeClient.AppsV1().Deployments(oteOperatorNamespace).Patch(
		ctx, oteOperandName, types.StrategicMergePatchType,
		[]byte(patch), metav1.PatchOptions{})
	if err != nil {
		t.Fatalf("Failed to patch deployment template annotation: %v", err)
	}

	expectedGen := genBefore + 1
	o.Eventually(func() error {
		dep, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Get(ctx, oteOperandName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get deployment: %v", err)
		}
		if dep.Generation != expectedGen {
			return fmt.Errorf("generation: want %d, got %d (operator may have reconciled)", expectedGen, dep.Generation)
		}
		if dep.Status.ObservedGeneration < expectedGen {
			return fmt.Errorf("rollout not yet complete: observedGeneration %d < %d", dep.Status.ObservedGeneration, expectedGen)
		}
		desiredReplicas := ptr.Deref(dep.Spec.Replicas, 0)
		if dep.Status.Replicas != desiredReplicas ||
			dep.Status.UpdatedReplicas != desiredReplicas ||
			dep.Status.AvailableReplicas != desiredReplicas {
			return fmt.Errorf("rollout not yet complete: replicas=%d updated=%d available=%d, want %d",
				dep.Status.Replicas, dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas, desiredReplicas)
		}
		for _, c := range dep.Status.Conditions {
			if c.Type == appsv1.DeploymentProgressing {
				if c.Status != corev1.ConditionTrue || c.Reason != "NewReplicaSetAvailable" {
					return fmt.Errorf("rollout not yet complete: Progressing condition has status=%s reason=%s, want True/NewReplicaSetAvailable", c.Status, c.Reason)
				}
				return nil
			}
		}
		return fmt.Errorf("rollout not yet complete: Progressing condition not found")
	}, 5*time.Minute, 5*time.Second).Should(o.Succeed(),
		"operator should not reconcile deployment when not managed")
}

// waitForOperatorAvailable waits for the operator to become Available with a fresh reconciliation,
// by checking that status.Generations has advanced beyond the supplied baseline.
func waitForOperatorAvailable(t testing.TB, ctx context.Context, jobSetOperatorClient jobsetoperatorv1clientset.JobSetOperatorInterface, genBefore int64) {
	t.Helper()

	o.Eventually(func() error {
		operator, err := jobSetOperatorClient.Get(ctx, "cluster", metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get operator: %v", err)
		}
		cond := v1helpers.FindOperatorCondition(operator.Status.Conditions, v1.OperatorStatusTypeAvailable)
		if cond == nil || cond.Status != v1.ConditionTrue {
			return fmt.Errorf("operator not yet available")
		}
		genAfter := operandDeploymentGeneration(operator.Status.Generations)
		if genAfter <= genBefore {
			return fmt.Errorf("status.Generations not yet updated: deployment generation %d (baseline %d)", genAfter, genBefore)
		}
		return nil
	}, 2*time.Minute, 5*time.Second).Should(o.Succeed(),
		"operator should become available with updated generations after restoring management state")
}

func operandDeploymentGeneration(generations []v1.GenerationStatus) int64 {
	for _, g := range generations {
		if g.Group == "apps" && g.Resource == "deployments" && g.Name == oteOperandName {
			return g.LastGeneration
		}
	}
	return 0
}
