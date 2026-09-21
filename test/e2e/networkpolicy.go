/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

//go:embed testdata/curl-test-pod.yaml
var curlPodTemplate string

//go:embed testdata/jobset-webhook-test.yaml
var jobsetWebhookTestYAML string

const (
	networkPolicyName = "jobset-allow-operand"
	operandPort       = 9443
	metricsPort       = 8443
)

var _ = Describe("[Operator][Serial] JobSet NetworkPolicy", Ordered, func() {
	var ctx context.Context

	BeforeAll(func() {
		ctx = context.Background()
	})

	// Test 1: Validate NetworkPolicy spec
	It("should create NetworkPolicy with correct spec", func() {
		expectNetworkPolicyValid(ctx, "NetworkPolicy should exist with correct spec")
	})

	// Test 2: Validate self-healing after mutation
	It("should reconcile NetworkPolicy after mutation", func() {
		netpolClient := clients.KubeClient.NetworkingV1().NetworkPolicies(operatorNamespace)

		By("Patching webhook port from 9443 to 1234")
		patch := []byte(`[{"op": "replace", "path": "/spec/ingress/0/ports/0/port", "value": 1234}]`)
		_, err := netpolClient.Patch(ctx, networkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to patch webhook port")

		expectNetworkPolicyValid(ctx, "operator should revert webhook port mutation")

		By("Tampering monitoring namespace selector")
		// Find the monitoring ingress rule index
		np, _ := netpolClient.Get(ctx, networkPolicyName, metav1.GetOptions{})
		monitoringIdx := len(np.Spec.Ingress) - 1 // Last ingress rule is monitoring
		patch = []byte(fmt.Sprintf(`[{"op": "replace", "path": "/spec/ingress/%d/from/0/namespaceSelector/matchLabels", "value": {"kubernetes.io/metadata.name": "fake-namespace"}}]`, monitoringIdx))
		_, err = netpolClient.Patch(ctx, networkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to patch monitoring selector")

		expectNetworkPolicyValid(ctx, "operator should revert monitoring selector mutation")

		By("Removing all egress rules")
		patch = []byte(`[{"op": "replace", "path": "/spec/egress", "value": []}]`)
		_, err = netpolClient.Patch(ctx, networkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to patch egress")

		expectNetworkPolicyValid(ctx, "operator should restore egress rules")
	})

	// Test 3: Validate self-healing after deletion
	It("should recreate NetworkPolicy after deletion", func() {
		netpolClient := clients.KubeClient.NetworkingV1().NetworkPolicies(operatorNamespace)

		By("Deleting NetworkPolicy")
		err := netpolClient.Delete(ctx, networkPolicyName, metav1.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to delete NetworkPolicy")

		expectNetworkPolicyValid(ctx, "operator should recreate NetworkPolicy after deletion")
	})

	// Test 4: Validate self-healing after operator restart
	It("should recover NetworkPolicy after config drift on operator restart", func() {
		netpolClient := clients.KubeClient.NetworkingV1().NetworkPolicies(operatorNamespace)

		By("Scaling down operator to 0 replicas")
		scaleDeployment(ctx, clients.KubeClient, operatorNamespace, "jobset-operator", 0)
		verifyPodCount(ctx, clients.KubeClient, operatorNamespace, "name=jobset-operator", 0)

		By("Wiping all ingress rules from NetworkPolicy")
		patch := []byte(`[{"op": "replace", "path": "/spec/ingress", "value": []}]`)
		_, err := netpolClient.Patch(ctx, networkPolicyName, types.JSONPatchType, patch, metav1.PatchOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to wipe ingress rules")

		np, err := netpolClient.Get(ctx, networkPolicyName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred(), "failed to get NetworkPolicy after wipe")
		Expect(np.Spec.Ingress).To(BeEmpty(), "expected 0 ingress rules after wipe")

		By("Scaling operator back up to 1 replica")
		scaleDeployment(ctx, clients.KubeClient, operatorNamespace, "jobset-operator", 1)

		Eventually(func() error {
			deploy, err := clients.KubeClient.AppsV1().Deployments(operatorNamespace).Get(ctx, "jobset-operator", metav1.GetOptions{})
			if err != nil {
				return err
			}
			if deploy.Status.ReadyReplicas < 1 {
				return fmt.Errorf("operator not ready yet: %d ready replicas", deploy.Status.ReadyReplicas)
			}
			return nil
		}, 2*time.Minute, 2*time.Second).Should(Succeed(), "operator should become ready")

		expectNetworkPolicyValid(ctx, "operator should recover NetworkPolicy after restart")
	})

	// Test 5: Traffic test - webhook port accessibility
	It("should allow webhook traffic on port 9443", func() {
		operandPod, err := getOperandPod(ctx, clients.KubeClient)
		Expect(err).NotTo(HaveOccurred(), "failed to get operand pod")
		webhookURL := fmt.Sprintf("https://%s:9443", operandPod.Status.PodIP)

		By("Testing webhook access from same namespace")
		code, err := runCurlPod(ctx, operatorNamespace, webhookURL)
		Expect(err).NotTo(HaveOccurred(), "curl from same namespace failed")
		Expect(code).NotTo(Equal("000"), "webhook port 9443 should be accessible from same namespace")

		By("Testing webhook access from default namespace")
		code, err = runCurlPod(ctx, "default", webhookURL)
		Expect(err).NotTo(HaveOccurred(), "curl from default namespace failed")
		Expect(code).NotTo(Equal("000"), "webhook port 9443 should be accessible from default namespace (unrestricted ingress)")
	})

	// Test 6: Traffic test - webhook via kube-apiserver
	It("should allow webhook via kube-apiserver", func() {
		By("Creating JobSet to test webhook admission via kube-apiserver")
		jobsetName, err := ocCreate(ctx, jobsetWebhookTestYAML)
		defer func() {
			_ = runCommand("oc", "delete", "jobset", jobsetName, "-n", "default", "--ignore-not-found")
		}()
		Expect(err).NotTo(HaveOccurred(), "webhook should allow JobSet creation")
	})

	// Test 7: Traffic test - metrics port access control
	It("should allow metrics from monitoring namespaces", func() {
		operandPod, err := getOperandPod(ctx, clients.KubeClient)
		Expect(err).NotTo(HaveOccurred(), "failed to get operand pod")
		metricsURL := fmt.Sprintf("https://%s:8443", operandPod.Status.PodIP)

		By("Testing metrics access from openshift-monitoring namespace")
		code, err := runCurlPod(ctx, "openshift-monitoring", metricsURL)
		Expect(err).NotTo(HaveOccurred(), "curl from openshift-monitoring failed")
		Expect(code).NotTo(Equal("000"), "metrics port 8443 should be accessible from openshift-monitoring")

		By("Testing metrics blocked from random namespace")
		code, err = runCurlPod(ctx, "default", metricsURL)
		Expect(err).NotTo(HaveOccurred(), "curl from default namespace failed")
		Expect(code).To(Equal("000"), "metrics port 8443 should be blocked from non-monitoring namespaces")
	})

	// Test 8: Traffic test - unlisted ports should be blocked
	It("should block traffic on unlisted port", func() {
		operandPod, err := getOperandPod(ctx, clients.KubeClient)
		Expect(err).NotTo(HaveOccurred(), "failed to get operand pod")

		By("Testing that unlisted port 8080 is blocked")
		code, err := runCurlPod(ctx, operatorNamespace, fmt.Sprintf("https://%s:8080", operandPod.Status.PodIP))
		Expect(err).NotTo(HaveOccurred(), "curl for unlisted port failed")
		Expect(code).To(Equal("000"), "unlisted port 8080 should be blocked")
	})

	// Test 9: Traffic test - egress connectivity
	It("should allow egress from operand to API server", func() {
		operandPod, err := getOperandPod(ctx, clients.KubeClient)
		Expect(err).NotTo(HaveOccurred(), "failed to get operand pod")
		Expect(operandPod.Status.Phase).To(Equal(corev1.PodRunning), "operand pod should be Running (requires egress to API server)")
	})
})

// expectNetworkPolicyValid waits for NetworkPolicy to exist and be valid
func expectNetworkPolicyValid(ctx context.Context, msg string) {
	Eventually(func() error {
		netpol, err := clients.KubeClient.NetworkingV1().NetworkPolicies(operatorNamespace).Get(ctx, networkPolicyName, metav1.GetOptions{})
		if err != nil {
			return err
		}
		return validateNetworkPolicySpec(netpol)
	}, 1*time.Minute, 2*time.Second).Should(Succeed(), msg)
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
func scaleDeployment(ctx context.Context, kubeClient *kubernetes.Clientset, namespace, name string, replicas int32) {
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
	_, err := kubeClient.AppsV1().Deployments(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "failed to scale deployment %s to %d replicas", name, replicas)
}

// verifyPodCount verifies that the expected number of pods exist with the given label selector
func verifyPodCount(ctx context.Context, kubeClient *kubernetes.Clientset, namespace, labelSelector string, expectedCount int) {
	EventuallyWithOffset(1, func() int {
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
	}, 3*time.Minute, 2*time.Second).Should(Equal(expectedCount),
		"expected %d pods with label %s in namespace %s", expectedCount, labelSelector, namespace)
}

// getOperandPod returns a running operand pod with an assigned IP
func getOperandPod(ctx context.Context, kubeClient *kubernetes.Clientset) (*corev1.Pod, error) {
	pods, err := kubeClient.CoreV1().Pods(operatorNamespace).List(ctx, metav1.ListOptions{
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

// runCommand executes a command and returns any error
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %v\n%s", name, args, err, string(out))
	}
	return nil
}
