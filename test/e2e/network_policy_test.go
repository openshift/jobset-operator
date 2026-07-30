/*
Copyright 2025.

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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"

	"github.com/openshift/jobset-operator/test/e2e/testutils"
)

const (
	defaultDenyPolicyName    = "default-deny"
	allowOperatorPolicyName  = "allow-operator"
	allowOperandPolicyName   = "allow-operand"
	reconcileTimeout         = 10 * time.Minute
)

var _ = Describe("JobSet Operator NetworkPolicy", Ordered, func() {

	It("should have default-deny policy", func() {
		ctx := context.TODO()

		By("Fetching default-deny NetworkPolicy")
		policy := testutils.GetNetworkPolicy(ctx, clients, operatorNamespace, defaultDenyPolicyName)

		By("Validating pod selector is empty (applies to all pods)")
		testutils.RequireEmptyPodSelector(policy)

		By("Validating no ingress rules (deny all ingress)")
		Expect(policy.Spec.Ingress).To(BeEmpty(), "default-deny should have no ingress rules")

		By("Validating no egress rules (deny all egress)")
		Expect(policy.Spec.Egress).To(BeEmpty(), "default-deny should have no egress rules")

		By("Validating policy types include both Ingress and Egress")
		Expect(policy.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeIngress))
		Expect(policy.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeEgress))
	})

	It("should have allow-operator policy with correct structure", func() {
		ctx := context.TODO()

		By("Fetching allow-operator NetworkPolicy")
		policy := testutils.GetNetworkPolicy(ctx, clients, operatorNamespace, allowOperatorPolicyName)

		By("Validating pod selector targets jobset-operator")
		testutils.RequirePodSelectorLabel(policy, "name", "jobset-operator")

		By("Validating ingress on port 8443 (metrics)")
		Expect(policy.Spec.Ingress).NotTo(BeEmpty(), "should have ingress rules")
		testutils.RequireIngressPort(policy, corev1.ProtocolTCP, 8443)

		By("Validating unrestricted egress")
		testutils.RequireUnrestrictedEgress(policy)

		By("Validating policy types include both Ingress and Egress")
		Expect(policy.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeIngress))
		Expect(policy.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeEgress))
	})

	It("should have allow-operand policy with correct structure", func() {
		ctx := context.TODO()

		By("Fetching allow-operand NetworkPolicy")
		policy := testutils.GetNetworkPolicy(ctx, clients, operatorNamespace, allowOperandPolicyName)

		By("Validating pod selector targets jobset operand")
		testutils.RequirePodSelectorLabel(policy, "app.kubernetes.io/name", "jobset")

		By("Validating ingress on port 8443 (metrics)")
		Expect(policy.Spec.Ingress).NotTo(BeEmpty(), "should have ingress rules")
		testutils.RequireIngressPort(policy, corev1.ProtocolTCP, 8443)

		By("Validating ingress on port 9443 (webhook)")
		testutils.RequireIngressPort(policy, corev1.ProtocolTCP, 9443)

		By("Validating unrestricted egress")
		testutils.RequireUnrestrictedEgress(policy)

		By("Validating policy types include both Ingress and Egress")
		Expect(policy.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeIngress))
		Expect(policy.Spec.PolicyTypes).To(ContainElement(networkingv1.PolicyTypeEgress))
	})

	It("should restore NetworkPolicies after deletion", func() {
		ctx := context.TODO()
		policyNames := []string{defaultDenyPolicyName, allowOperatorPolicyName, allowOperandPolicyName}

		for _, name := range policyNames {
			By("Capturing and restoring " + name)
			expected := testutils.GetNetworkPolicy(ctx, clients, operatorNamespace, name)
			testutils.RestoreNetworkPolicy(ctx, clients, expected, reconcileTimeout)
		}
	})

	It("should restore NetworkPolicies after mutation", func() {
		ctx := context.TODO()
		policyNames := []string{defaultDenyPolicyName, allowOperatorPolicyName, allowOperandPolicyName}

		for _, name := range policyNames {
			By("Mutating and waiting for reconciliation of " + name)
			testutils.MutateAndRestoreNetworkPolicy(ctx, clients, operatorNamespace, name, reconcileTimeout)
		}
	})
})
