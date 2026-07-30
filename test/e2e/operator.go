package e2e

import (
	"context"
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

	certManagerURL = "https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml"
)

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
	waitForManagementStateProcessed(t, ctx, kubeClient, jobSetOperatorClient, v1.Unmanaged)
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
	waitForManagementStateProcessed(t, ctx, kubeClient, jobSetOperatorClient, v1.Removed)
	scaleDeployment(t, ctx, kubeClient, oteOperandName, 3)
	verifyPodCount(t, ctx, kubeClient, oteOperatorNamespace, oteOperandLabel, 3)
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

// waitForManagementStateProcessed waits until the reconciler has processed a
// management-state change so that it will not overwrite a subsequent manual
// scale.
//
// A direct API poll only confirms the etcd write; the reconciler reads from
// its informer cache, which may lag. To close the race, this function:
//
//  1. Confirms the CR reflects the expected management state (API propagation).
//  2. Patches a disposable annotation on the operand deployment, which forces
//     the deployment informer to fire and enqueue a sync in the reconciler.
//  3. Waits for the deployment generation to stabilise. If the reconciler ran
//     with stale Managed state it will overwrite the deployment spec (bumping
//     the generation); the informer eventually catches up and subsequent syncs
//     become no-ops. Once the generation is stable, the reconciler has
//     converged on the new management state.
func waitForManagementStateProcessed(t testing.TB, ctx context.Context, kubeClient *k8sclient.Clientset, jobSetOperatorClient jobsetoperatorv1clientset.JobSetOperatorInterface, expectedState v1.ManagementState) {
	t.Helper()

	o.Eventually(func() v1.ManagementState {
		operator, err := jobSetOperatorClient.Get(ctx, "cluster", metav1.GetOptions{})
		if err != nil {
			return ""
		}
		return operator.Spec.ManagementState
	}, 30*time.Second, 1*time.Second).Should(o.Equal(expectedState),
		"operator CR should reflect management state %s", expectedState)

	syncTrigger := fmt.Sprintf(`{"metadata":{"annotations":{"test.openshift.io/sync-trigger":"%s"}}}`,
		time.Now().Format(time.RFC3339Nano))
	_, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Patch(
		ctx, oteOperandName, types.StrategicMergePatchType,
		[]byte(syncTrigger), metav1.PatchOptions{})
	if err != nil {
		t.Fatalf("Failed to patch deployment to trigger sync: %v", err)
	}

	var lastGen int64
	stable := 0
	o.Eventually(func() bool {
		dep, err := kubeClient.AppsV1().Deployments(oteOperatorNamespace).Get(ctx, oteOperandName, metav1.GetOptions{})
		if err != nil {
			return false
		}
		if dep.Generation == lastGen {
			stable++
		} else {
			lastGen = dep.Generation
			stable = 0
		}
		return stable >= 3
	}, 60*time.Second, 2*time.Second).Should(o.BeTrue(),
		"deployment generation should stabilize after management state change")
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
