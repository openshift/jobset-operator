/*
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

// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	openshiftoperatorv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"
	applyconfigurationopenshiftoperatorv1 "github.com/openshift/jobset-operator/pkg/generated/applyconfiguration/openshiftoperator/v1"
	scheme "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// JobSetOperatorsGetter has a method to return a JobSetOperatorInterface.
// A group's client should implement this interface.
type JobSetOperatorsGetter interface {
	JobSetOperators(namespace string) JobSetOperatorInterface
}

// JobSetOperatorInterface has methods to work with JobSetOperator resources.
type JobSetOperatorInterface interface {
	Create(ctx context.Context, jobSetOperator *openshiftoperatorv1.JobSetOperator, opts metav1.CreateOptions) (*openshiftoperatorv1.JobSetOperator, error)
	Update(ctx context.Context, jobSetOperator *openshiftoperatorv1.JobSetOperator, opts metav1.UpdateOptions) (*openshiftoperatorv1.JobSetOperator, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, jobSetOperator *openshiftoperatorv1.JobSetOperator, opts metav1.UpdateOptions) (*openshiftoperatorv1.JobSetOperator, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*openshiftoperatorv1.JobSetOperator, error)
	List(ctx context.Context, opts metav1.ListOptions) (*openshiftoperatorv1.JobSetOperatorList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *openshiftoperatorv1.JobSetOperator, err error)
	Apply(ctx context.Context, jobSetOperator *applyconfigurationopenshiftoperatorv1.JobSetOperatorApplyConfiguration, opts metav1.ApplyOptions) (result *openshiftoperatorv1.JobSetOperator, err error)
	// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
	ApplyStatus(ctx context.Context, jobSetOperator *applyconfigurationopenshiftoperatorv1.JobSetOperatorApplyConfiguration, opts metav1.ApplyOptions) (result *openshiftoperatorv1.JobSetOperator, err error)
	JobSetOperatorExpansion
}

// jobSetOperators implements JobSetOperatorInterface
type jobSetOperators struct {
	*gentype.ClientWithListAndApply[*openshiftoperatorv1.JobSetOperator, *openshiftoperatorv1.JobSetOperatorList, *applyconfigurationopenshiftoperatorv1.JobSetOperatorApplyConfiguration]
}

// newJobSetOperators returns a JobSetOperators
func newJobSetOperators(c *OpenShiftOperatorV1Client, namespace string) *jobSetOperators {
	return &jobSetOperators{
		gentype.NewClientWithListAndApply[*openshiftoperatorv1.JobSetOperator, *openshiftoperatorv1.JobSetOperatorList, *applyconfigurationopenshiftoperatorv1.JobSetOperatorApplyConfiguration](
			"jobsetoperators",
			c.RESTClient(),
			scheme.ParameterCodec,
			namespace,
			func() *openshiftoperatorv1.JobSetOperator { return &openshiftoperatorv1.JobSetOperator{} },
			func() *openshiftoperatorv1.JobSetOperatorList { return &openshiftoperatorv1.JobSetOperatorList{} },
		),
	}
}
