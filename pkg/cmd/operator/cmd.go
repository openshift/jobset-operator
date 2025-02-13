package operator

import (
	"context"

	"github.com/spf13/cobra"

	"k8s.io/utils/clock"

	"github.com/openshift/library-go/pkg/controller/controllercmd"

	"github.com/openshift/jobset-operator/pkg/operator"
	"github.com/openshift/jobset-operator/pkg/version"
)

func NewOperator() *cobra.Command {
	cmd := controllercmd.
		NewControllerCommandConfig("openshift-jobset-operator", version.Get(), operator.RunOperator, clock.RealClock{}).
		NewCommandWithContext(context.TODO())
	cmd.Use = "operator"
	cmd.Short = "Start the cluster JobSet operator"

	return cmd
}
