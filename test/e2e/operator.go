package e2e

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	v1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/jobset-operator/bindata"
	"github.com/openshift/jobset-operator/deploy"
	operatorv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"
	jobsetoperatorv1clientset "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned/typed/openshiftoperator/v1"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	oteNetworkPolicyName = "jobset-operand"

	certManagerURL = "https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml"
)

var deployTmpDir string

var jobSetGVR = schema.GroupVersionResource{Group: "jobset.x-k8s.io", Version: "v1alpha2", Resource: "jobsets"}

//go:embed testdata/test-jobset-networkpolicy-webhook-traffic.yaml
var testJobSetNetworkPolicyWebhookTrafficYAML string

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

	g.It("should not reconcile when managementState is Unmanaged [Suite:openshift/jobset-operator/operator/serial]", func() {
		testUnmanagedState(g.GinkgoTB(), ctx, kubeClient)
	})

	g.It("should not reconcile when managementState is Removed [Suite:openshift/jobset-operator/operator/serial]", func() {
		testRemovedState(g.GinkgoTB(), ctx, kubeClient)
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
		// Get the NetworkPolicy to find the monitoring ingress rule index
		np, err := netpolClient.Get(ctx, oteNetworkPolicyName, metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get NetworkPolicy")
		monitoringIdx := len(np.Spec.Ingress) - 1 // Last ingress rule is monitoring
		patch = []byte(fmt.Sprintf(`[{"op": "replace", "path": "/spec/ingress/%d/from/0/namespaceSelector/matchLabels", "value": {"kubernetes.io/metadata.name": "fake-namespace"}}]`, monitoringIdx))
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

	g.It("should allow webhook traffic on port 9443 from operator namespace and block unlisted port [Suite:openshift/jobset-operator/operator/serial]", func() {
		operandPod, err := getOperandPod(ctx, kubeClient)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get operand pod")

		// Port 9443 allows all ingress (kube-apiserver uses host network; cannot restrict by namespace).
		// Verify it is reachable, and that an unlisted port is blocked from the same source.
		webhookURL := fmt.Sprintf("https://%s:9443", operandPod.Status.PodIP)
		code, err := runCurlPod(ctx, kubeClient, oteOperatorNamespace, webhookURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl on port 9443 failed")
		o.Expect(code).To(o.Equal(strconv.Itoa(http.StatusNotFound)), "webhook port 9443 should return HTTP 404 from the operator namespace")

		blockedURL := fmt.Sprintf("https://%s:1234", operandPod.Status.PodIP)
		code, err = runCurlPod(ctx, kubeClient, oteOperatorNamespace, blockedURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl on unlisted port failed")
		o.Expect(code).To(o.Equal("000"), "unlisted port 1234 should be blocked from the operator namespace")
	})

	g.It("should allow webhook via kube-apiserver and operand egress to API server [Suite:openshift/jobset-operator/operator/serial]", func() {
		// Test both webhook functionality and operand egress to API server.
		// If egress to the API server is blocked, the webhook endpoint would fail to validate JobSets.
		// Successful JobSet creation proves the webhook can communicate with the API server.
		klog.Infof("Creating JobSet to test webhook via kube-apiserver (validates egress)")
		jobSetClient := GetDynamicClient().Resource(jobSetGVR).Namespace(metav1.NamespaceDefault)
		jobset, err := jobSetClient.Create(ctx, resourceread.ReadUnstructuredOrDie([]byte(testJobSetNetworkPolicyWebhookTrafficYAML)), metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "webhook rejected JobSet creation (egress to API server may be blocked)")
		defer func() {
			err := jobSetClient.Delete(ctx, jobset.GetName(), metav1.DeleteOptions{})
			o.Expect(err).NotTo(o.HaveOccurred(), "failed to delete webhook test JobSet")
		}()
		klog.Infof("JobSet %s created successfully via webhook (confirms egress to API server)", jobset.GetName())
	})

	g.It("should allow metrics from monitoring and block from random namespace [Suite:openshift/jobset-operator/operator/serial]", func() {
		operandPod, err := getOperandPod(ctx, kubeClient)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to get operand pod")
		metricsURL := fmt.Sprintf("https://%s:8443", operandPod.Status.PodIP)

		// Create a random namespace (no cluster-monitoring label) to test that it is blocked.
		blockNS, err := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{GenerateName: "test-netpol-e2e-"},
		}, metav1.CreateOptions{})
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to create test namespace")
		defer func() {
			err := kubeClient.CoreV1().Namespaces().Delete(ctx, blockNS.Name, metav1.DeleteOptions{})
			o.Expect(err).NotTo(o.HaveOccurred(), "failed to delete test namespace")
		}()

		// openshift-monitoring carries the cluster-monitoring label and must be allowed.
		code, err := runCurlPod(ctx, kubeClient, "openshift-monitoring", metricsURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl from openshift-monitoring failed")
		o.Expect(code).To(o.Equal(strconv.Itoa(http.StatusNotFound)), "metrics port 8443 should return HTTP 404 from openshift-monitoring")

		// The unlabelled random namespace must be blocked.
		code, err = runCurlPod(ctx, kubeClient, blockNS.Name, metricsURL)
		o.Expect(err).NotTo(o.HaveOccurred(), "curl from random namespace failed")
		o.Expect(code).To(o.Equal("000"), "metrics port 8443 should be blocked from random namespace")
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
		err := os.RemoveAll(deployTmpDir)
		o.Expect(err).NotTo(o.HaveOccurred(), "failed to remove temporary deploy manifests")
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

func testUnmanagedState(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	jobSetOperatorClient := GetJobSetOperatorClient()

	jobsetOperator, originalState, err := getOperatorState(ctx, jobSetOperatorClient)
	if err != nil {
		t.Fatalf("Failed to get operator state: %v", err)
	}

	var genBaseline int64
	defer func() {
		setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, originalState)
		waitForOperatorAvailable(t, ctx, jobSetOperatorClient, genBaseline)
		expectNetworkPolicyValid(ctx, kubeClient, "operator should restore the NetworkPolicy after restoring management state")
	}()

	setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, v1.Unmanaged)
	// Give the operator a chance to finish an ongoing sync.
	time.Sleep(1 * time.Second)
	genBaseline = operandDeploymentGeneration(jobsetOperator.Status.Generations)
	verifyOperatorDoesNotReconcile(t, ctx, kubeClient)
	verifyNetworkPolicyNotReconciled(t, ctx, kubeClient)
}

func testRemovedState(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	jobSetOperatorClient := GetJobSetOperatorClient()

	jobsetOperator, originalState, err := getOperatorState(ctx, jobSetOperatorClient)
	if err != nil {
		t.Fatalf("Failed to get operator state: %v", err)
	}

	var genBaseline int64
	defer func() {
		newctx := context.TODO()
		setManagementState(t, newctx, jobSetOperatorClient, jobsetOperator, originalState)
		waitForOperatorAvailable(t, newctx, jobSetOperatorClient, genBaseline)
		expectNetworkPolicyValid(newctx, kubeClient, "operator should restore the NetworkPolicy after restoring management state")
	}()

	setManagementState(t, ctx, jobSetOperatorClient, jobsetOperator, v1.Removed)
	// Give the operator a chance to finish an ongoing sync
	time.Sleep(1 * time.Second)
	genBaseline = operandDeploymentGeneration(jobsetOperator.Status.Generations)
	verifyOperatorDoesNotReconcile(t, ctx, kubeClient)
	verifyNetworkPolicyNotReconciled(t, ctx, kubeClient)
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

// verifyNetworkPolicyNotReconciled deletes the NetworkPolicy and confirms that the operator does
// not recreate it while the management state prevents reconciliation.
func verifyNetworkPolicyNotReconciled(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset) {
	t.Helper()
	netpolClient := kubeClient.NetworkingV1().NetworkPolicies(oteOperatorNamespace)

	klog.Infof("Deleting NetworkPolicy %s to verify operator does not reconcile it", oteNetworkPolicyName)
	err := netpolClient.Delete(ctx, oteNetworkPolicyName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		t.Fatalf("Failed to delete NetworkPolicy: %v", err)
	}

	// The operator should not recreate the NetworkPolicy while in Unmanaged/Removed state.
	o.ConsistentlyWithOffset(1, func() error {
		_, getErr := netpolClient.Get(ctx, oteNetworkPolicyName, metav1.GetOptions{})
		if k8serrors.IsNotFound(getErr) {
			return nil // expected: still absent
		}
		if getErr != nil {
			return getErr
		}
		return fmt.Errorf("NetworkPolicy was unexpectedly recreated by the operator")
	}, 5*time.Second, 2*time.Second).Should(o.Succeed(),
		"operator should not recreate NetworkPolicy when management state prevents reconciliation")
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

// expectNetworkPolicyValid waits for the NetworkPolicy to exist and match the expected shape.
func expectNetworkPolicyValid(ctx context.Context, kubeClient *k8sclient.Clientset, msg string) {
	expected := resourceread.ReadNetworkPolicyV1OrDie(
		bindata.MustAsset("assets/jobset-controller/allow-operand-networkpolicy.yaml"),
	)

	o.Eventually(func() (*networkingv1.NetworkPolicy, error) {
		return kubeClient.NetworkingV1().NetworkPolicies(oteOperatorNamespace).Get(
			ctx, oteNetworkPolicyName, metav1.GetOptions{})
	}, 1*time.Minute, 2*time.Second).Should(o.And(
		o.HaveField("Spec", o.Equal(expected.Spec)),
		o.HaveField("OwnerReferences", o.ContainElement(o.And(
			o.HaveField("APIVersion", "operator.openshift.io/v1"),
			o.HaveField("Kind", "JobSetOperator"),
			o.HaveField("Name", "cluster"),
		))),
	), msg)
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

// runCurlPod creates a curl test pod using client-go and returns the HTTP status code string
// written to stdout by curl (e.g. "200", "000" for a blocked/timed-out connection).
// The pod is deleted automatically when the function returns.
func runCurlPod(ctx context.Context, kubeClient *k8sclient.Clientset, namespace, targetURL string) (code string, retErr error) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "netpol-curl-test-",
			Namespace:    namespace,
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{
				{
					Name:  "curl",
					Image: "curlimages/curl:8.13.0",
					// -s: silent  -k: skip TLS verification  --connect-timeout: give up early when blocked
					// -o /dev/null -w "%{http_code}": print only the HTTP status code to stdout.
					// A NetworkPolicy drop will cause curl to time out and print "000".
					Command: []string{
						"curl", "-sk", "--connect-timeout", "5",
						"-o", "/dev/null", "-w", "%{http_code}",
						targetURL,
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			},
		},
	}

	created, err := kubeClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create curl pod: %v", err)
	}
	defer func() {
		deleteErr := kubeClient.CoreV1().Pods(namespace).Delete(ctx, created.Name,
			metav1.DeleteOptions{GracePeriodSeconds: ptr.To(int64(0))})
		if deleteErr != nil && !k8serrors.IsNotFound(deleteErr) && retErr == nil {
			retErr = fmt.Errorf("failed to delete curl pod: %w", deleteErr)
		}
	}()

	// Wait for the pod to reach a terminal phase (Succeeded or Failed).
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 90*time.Second, true,
		func(ctx context.Context) (bool, error) {
			p, getErr := kubeClient.CoreV1().Pods(namespace).Get(ctx, created.Name, metav1.GetOptions{})
			if getErr != nil {
				return false, getErr
			}
			return p.Status.Phase == corev1.PodSucceeded || p.Status.Phase == corev1.PodFailed, nil
		})
	if err != nil {
		return "", fmt.Errorf("curl pod did not reach a terminal phase: %v", err)
	}

	// Retrieve the HTTP status code from the pod's log output.
	logStream, err := kubeClient.CoreV1().Pods(namespace).GetLogs(created.Name, &corev1.PodLogOptions{}).Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to stream curl pod logs: %v", err)
	}
	defer func() {
		if closeErr := logStream.Close(); closeErr != nil && retErr == nil {
			retErr = fmt.Errorf("failed to close curl pod log stream: %w", closeErr)
		}
	}()

	var buf bytes.Buffer
	if _, err = io.Copy(&buf, logStream); err != nil {
		return "", fmt.Errorf("failed to read curl pod logs: %v", err)
	}
	return strings.TrimSpace(buf.String()), nil
}
