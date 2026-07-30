package testutils

import (
	"context"
	"fmt"
	"time"

	"github.com/onsi/gomega"
	v1 "github.com/openshift/api/operator/v1"
	operatorv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

const (
	operatorNamespace = "openshift-jobset-operator"
	OperandName       = "jobset-controller-manager"
)

func GetOperatorState(ctx context.Context, clients *TestClients) (*operatorv1.JobSetOperator, v1.ManagementState, error) {
	if clients == nil || clients.JobSetOperatorClient == nil {
		return nil, "", fmt.Errorf("nil clients or LWSOperatorClient")
	}
	jobsetOperator, err := clients.JobSetOperatorClient.Get(ctx, "cluster", metav1.GetOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to get operator: %w", err)
	}

	return jobsetOperator, jobsetOperator.Spec.ManagementState, nil
}

func SetManagementState(ctx context.Context, clients *TestClients, operator *operatorv1.JobSetOperator, state v1.ManagementState) {
	retryErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		current, getErr := clients.JobSetOperatorClient.Get(ctx, operator.Name, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}
		current.Spec.ManagementState = state
		_, updateErr := clients.JobSetOperatorClient.Update(ctx, current, metav1.UpdateOptions{})
		return updateErr
	})
	gomega.Expect(retryErr).NotTo(gomega.HaveOccurred(), "failed to update operator state after retries")
}

func ScaleDeployment(ctx context.Context, clients *TestClients, OperandName string, replicas int32) {
	patch := fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas)
	_, err := clients.KubeClient.AppsV1().Deployments(operatorNamespace).Patch(
		ctx,
		OperandName,
		types.StrategicMergePatchType,
		[]byte(patch),
		metav1.PatchOptions{})
	if err != nil {
		klog.Errorf("WARNING: Failed to restore replicas: %v\n", err)
	}
}

func VerifyPodCount(ctx context.Context, clients *TestClients, namespace, labelSelector string, expected int) {
	gomega.Eventually(func() int {
		return GetPodCount(ctx, clients, namespace, labelSelector)
	}, 5*time.Minute, 10*time.Second).Should(
		gomega.Equal(expected),
		"Pod count should reach %d", expected)
}

func GetPodCount(ctx context.Context, clients *TestClients, namespace, labelSelector string) int {
	pods, err := clients.KubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		klog.Errorf("Pod list error: %v\n", err)
		return -1
	}
	return len(pods.Items)
}

func GetNetworkPolicy(ctx context.Context, clients *TestClients, namespace, name string) *networkingv1.NetworkPolicy {
	policy, err := clients.KubeClient.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to get NetworkPolicy %s/%s", namespace, name)
	return policy
}

func RequirePodSelectorLabel(policy *networkingv1.NetworkPolicy, key, value string) {
	actual, ok := policy.Spec.PodSelector.MatchLabels[key]
	gomega.Expect(ok).To(gomega.BeTrue(), "%s/%s: podSelector missing label %s", policy.Namespace, policy.Name, key)
	gomega.Expect(actual).To(gomega.Equal(value), "%s/%s: podSelector label %s", policy.Namespace, policy.Name, key)
}

func RequireEmptyPodSelector(policy *networkingv1.NetworkPolicy) {
	gomega.Expect(policy.Spec.PodSelector.MatchLabels).To(gomega.BeEmpty(),
		"%s/%s: expected empty podSelector matchLabels", policy.Namespace, policy.Name)
	gomega.Expect(policy.Spec.PodSelector.MatchExpressions).To(gomega.BeEmpty(),
		"%s/%s: expected empty podSelector matchExpressions", policy.Namespace, policy.Name)
}

func RequireIngressPort(policy *networkingv1.NetworkPolicy, protocol corev1.Protocol, port int32) {
	found := false
	for _, rule := range policy.Spec.Ingress {
		for _, p := range rule.Ports {
			if p.Protocol != nil && *p.Protocol == protocol && p.Port != nil && p.Port.IntValue() == int(port) {
				found = true
				break
			}
			if p.Protocol == nil && protocol == corev1.ProtocolTCP && p.Port != nil && p.Port.IntValue() == int(port) {
				found = true
				break
			}
		}
	}
	gomega.Expect(found).To(gomega.BeTrue(), "%s/%s: expected ingress port %s/%d", policy.Namespace, policy.Name, protocol, port)
}

func RequireUnrestrictedEgress(policy *networkingv1.NetworkPolicy) {
	gomega.Expect(policy.Spec.Egress).NotTo(gomega.BeEmpty(),
		"%s/%s: expected at least one egress rule", policy.Namespace, policy.Name)
	found := false
	for _, rule := range policy.Spec.Egress {
		if len(rule.Ports) == 0 && len(rule.To) == 0 {
			found = true
			break
		}
	}
	gomega.Expect(found).To(gomega.BeTrue(),
		"%s/%s: no unrestricted egress rule [{}] found", policy.Namespace, policy.Name)
}

func RestoreNetworkPolicy(ctx context.Context, clients *TestClients, expected *networkingv1.NetworkPolicy, timeout time.Duration) {
	namespace := expected.Namespace
	name := expected.Name
	klog.Infof("Deleting NetworkPolicy %s/%s and waiting for restoration", namespace, name)
	err := clients.KubeClient.NetworkingV1().NetworkPolicies(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to delete NetworkPolicy %s/%s", namespace, name)

	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		current, err := clients.KubeClient.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return equality.Semantic.DeepEqual(expected.Spec, current.Spec), nil
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "timed out waiting for NetworkPolicy %s/%s to be restored", namespace, name)
	klog.Infof("NetworkPolicy %s/%s restored after delete", namespace, name)
}

func MutateAndRestoreNetworkPolicy(ctx context.Context, clients *TestClients, namespace, name string, timeout time.Duration) {
	original := GetNetworkPolicy(ctx, clients, namespace, name)
	klog.Infof("Mutating NetworkPolicy %s/%s and waiting for reconciliation", namespace, name)

	patch := []byte(`{"spec":{"podSelector":{"matchLabels":{"np-reconcile":"mutated"}}}}`)
	_, err := clients.KubeClient.NetworkingV1().NetworkPolicies(namespace).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "failed to patch NetworkPolicy %s/%s", namespace, name)

	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		current, err := clients.KubeClient.NetworkingV1().NetworkPolicies(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		return equality.Semantic.DeepEqual(original.Spec, current.Spec), nil
	})
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "timed out waiting for NetworkPolicy %s/%s to be restored after mutation", namespace, name)
	klog.Infof("NetworkPolicy %s/%s restored after mutation", namespace, name)
}
