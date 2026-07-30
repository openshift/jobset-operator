package operator

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/clock"

	"github.com/openshift/jobset-operator/pkg/operator/operatorclient"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
)

func TestManageNetworkPolicyOperandAllow(t *testing.T) {
	ctx := context.Background()
	kubeClient := fake.NewSimpleClientset()
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})
	testNamespace := "test-namespace"

	reconciler := &TargetConfigReconciler{
		kubeClient:        kubeClient,
		eventRecorder:     recorder,
		operatorNamespace: testNamespace,
		resourceCache:     resourceapply.NewResourceCache(),
	}

	ownerReference := metav1.OwnerReference{
		APIVersion: "operator.openshift.io/v1",
		Kind:       "JobSetOperator",
		Name:       operatorclient.OperatorConfigName,
		UID:        "test-uid",
	}

	netpol, modified, err := reconciler.manageNetworkPolicyOperandAllow(ctx, ownerReference)
	if err != nil {
		t.Fatalf("failed to apply network policy: %v", err)
	}
	if !modified {
		t.Fatal("expected modified=true on first apply")
	}
	if netpol.Name != "jobset-allow-operand" {
		t.Errorf("expected name %q, got %q", "jobset-allow-operand", netpol.Name)
	}
	if netpol.Namespace != testNamespace {
		t.Errorf("expected namespace %q, got %q", testNamespace, netpol.Namespace)
	}

	t.Run("second apply is idempotent", func(t *testing.T) {
		_, modified, err := reconciler.manageNetworkPolicyOperandAllow(ctx, ownerReference)
		if err != nil {
			t.Fatalf("second apply should not error, got: %v", err)
		}
		if modified {
			t.Error("expected modified=false on second apply (no changes)")
		}
	})
}
