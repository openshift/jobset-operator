package e2e

import (
	"testing"

	o "github.com/onsi/gomega"
)

func TestExtended(t *testing.T) {
	o.RegisterTestingT(t)
	ctx, cancelFnc, kubeClient, err := setupOperator(t)
	if err != nil {
		t.Fatalf("Failed to setup operator: %v", err)
	}
	defer func() {
		teardownOperator()
		cancelFnc()
	}()

	t.Run("should have correct operator conditions", func(t *testing.T) {
		testOperatorConditions(t, ctx, kubeClient)
	})

	t.Run("should recover operand pods after deletion", func(t *testing.T) {
		testOperandPodRecovery(t, ctx, kubeClient)
	})

	t.Run("should allow manual scaling when managementState is Unmanaged", func(t *testing.T) {
		testUnmanagedScaling(t, ctx, kubeClient)
	})

	t.Run("should keep operand scaled when managementState is Removed", func(t *testing.T) {
		testRemovedStateScaling(t, ctx, kubeClient)
	})
}
