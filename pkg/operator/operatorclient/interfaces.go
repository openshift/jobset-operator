package operatorclient

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/clock"

	operatorv1 "github.com/openshift/api/operator/v1"
	applyconfiguration "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	"github.com/openshift/library-go/pkg/apiserver/jsonpatch"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	apisopenshiftoperatorv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"
	jobsetoperatorapplyconfiguration "github.com/openshift/jobset-operator/pkg/generated/applyconfiguration/openshiftoperator/v1"
	operatorconfigclientv1 "github.com/openshift/jobset-operator/pkg/generated/clientset/versioned/typed/openshiftoperator/v1"
	openshiftoperatorv1 "github.com/openshift/jobset-operator/pkg/generated/listers/openshiftoperator/v1"
)

const OperatorConfigName = "cluster"
const OperandName = "jobset-controller-manager"

var _ v1helpers.OperatorClient = &JobSetOperatorClient{}

type JobSetOperatorClient struct {
	Ctx            context.Context
	SharedInformer cache.SharedIndexInformer
	Lister         openshiftoperatorv1.JobSetOperatorLister
	OperatorClient operatorconfigclientv1.OpenShiftOperatorV1Interface
}

func (j JobSetOperatorClient) Informer() cache.SharedIndexInformer {
	return j.SharedInformer
}

func (j JobSetOperatorClient) GetOperatorState() (spec *operatorv1.OperatorSpec, status *operatorv1.OperatorStatus, resourceVersion string, err error) {
	if !j.SharedInformer.HasSynced() {
		return j.GetOperatorStateWithQuorum(j.Ctx)
	}
	instance, err := j.Lister.Get(OperatorConfigName)
	if err != nil {
		return nil, nil, "", err
	}
	return &instance.Spec.OperatorSpec, &instance.Status.OperatorStatus, instance.ResourceVersion, nil
}

func (j JobSetOperatorClient) GetOperatorStateWithQuorum(ctx context.Context) (*operatorv1.OperatorSpec, *operatorv1.OperatorStatus, string, error) {
	instance, err := j.OperatorClient.JobSetOperators().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, "", err
	}
	return &instance.Spec.OperatorSpec, &instance.Status.OperatorStatus, instance.ResourceVersion, nil
}

func (j *JobSetOperatorClient) UpdateOperatorSpec(ctx context.Context, resourceVersion string, spec *operatorv1.OperatorSpec) (out *operatorv1.OperatorSpec, newResourceVersion string, err error) {
	original, err := j.OperatorClient.JobSetOperators().Get(ctx, OperatorConfigName, metav1.GetOptions{ResourceVersion: resourceVersion})
	if err != nil {
		return nil, "", err
	}
	original.Spec.OperatorSpec = *spec

	ret, err := j.OperatorClient.JobSetOperators().Update(ctx, original, v1.UpdateOptions{})
	if err != nil {
		return nil, "", err
	}

	return &ret.Spec.OperatorSpec, ret.ResourceVersion, nil
}

func (j *JobSetOperatorClient) UpdateOperatorStatus(ctx context.Context, resourceVersion string, status *operatorv1.OperatorStatus) (out *operatorv1.OperatorStatus, err error) {
	original, err := j.OperatorClient.JobSetOperators().Get(ctx, OperatorConfigName, metav1.GetOptions{ResourceVersion: resourceVersion})
	if err != nil {
		return nil, err
	}
	original.Status.OperatorStatus = *status

	ret, err := j.OperatorClient.JobSetOperators().UpdateStatus(ctx, original, v1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return &ret.Status.OperatorStatus, nil
}

func (j *JobSetOperatorClient) GetObjectMeta() (meta *metav1.ObjectMeta, err error) {
	var instance *apisopenshiftoperatorv1.JobSetOperator
	if j.SharedInformer.HasSynced() {
		instance, err = j.Lister.Get(OperatorConfigName)
		if err != nil {
			return nil, err
		}
	} else {
		instance, err = j.OperatorClient.JobSetOperators().Get(j.Ctx, OperatorConfigName, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
	return &instance.ObjectMeta, nil
}

func (j *JobSetOperatorClient) ApplyOperatorSpec(ctx context.Context, fieldManager string, desiredConfiguration *applyconfiguration.OperatorSpecApplyConfiguration) error {
	if desiredConfiguration == nil {
		return fmt.Errorf("applyConfiguration must have a value")
	}

	desiredSpec := &jobsetoperatorapplyconfiguration.JobSetOperatorSpecApplyConfiguration{
		OperatorSpecApplyConfiguration: *desiredConfiguration,
	}
	desired := jobsetoperatorapplyconfiguration.JobSetOperator(OperatorConfigName)
	desired.WithSpec(desiredSpec)

	instance, err := j.OperatorClient.JobSetOperators().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
	// do nothing and proceed with the apply
	case err != nil:
		return fmt.Errorf("unable to get operator configuration: %w", err)
	default:
		original, err := jobsetoperatorapplyconfiguration.ExtractJobSetOperator(instance, fieldManager)
		if err != nil {
			return fmt.Errorf("unable to extract operator configuration from spec: %w", err)
		}
		if equality.Semantic.DeepEqual(original, desired) {
			return nil
		}
	}

	_, err = j.OperatorClient.JobSetOperators().Apply(ctx, desired, v1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	})
	if err != nil {
		return fmt.Errorf("unable to Apply for operator using fieldManager %q: %w", fieldManager, err)
	}

	return nil
}

func (j *JobSetOperatorClient) ApplyOperatorStatus(ctx context.Context, fieldManager string, desiredConfiguration *applyconfiguration.OperatorStatusApplyConfiguration) error {
	if desiredConfiguration == nil {
		return fmt.Errorf("applyConfiguration must have a value")
	}

	desiredStatus := &jobsetoperatorapplyconfiguration.JobSetOperatorStatusApplyConfiguration{
		OperatorStatusApplyConfiguration: *desiredConfiguration,
	}
	desired := jobsetoperatorapplyconfiguration.JobSetOperator(OperatorConfigName)
	desired.WithStatus(desiredStatus)

	instance, err := j.OperatorClient.JobSetOperators().Get(ctx, OperatorConfigName, metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		// do nothing and proceed with the apply
		v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desired.Status.Conditions, nil)
	case err != nil:
		return fmt.Errorf("unable to get operator configuration: %w", err)
	default:
		original, err := jobsetoperatorapplyconfiguration.ExtractJobSetOperatorStatus(instance, fieldManager)
		if err != nil {
			return fmt.Errorf("unable to extract operator configuration from status: %w", err)
		}
		if equality.Semantic.DeepEqual(original, desired) {
			return nil
		}
		if original.Status != nil {
			v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desired.Status.Conditions, original.Status.Conditions)
		} else {
			v1helpers.SetApplyConditionsLastTransitionTime(clock.RealClock{}, &desired.Status.Conditions, nil)
		}
	}

	_, err = j.OperatorClient.JobSetOperators().ApplyStatus(ctx, desired, v1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	})
	if err != nil {
		return fmt.Errorf("unable to ApplyStatus for operator using fieldManager %q: %w", fieldManager, err)
	}

	return nil
}

func (j JobSetOperatorClient) PatchOperatorStatus(ctx context.Context, jsonPatch *jsonpatch.PatchSet) (err error) {
	jsonPatchBytes, err := jsonPatch.Marshal()
	if err != nil {
		return err
	}
	_, err = j.OperatorClient.JobSetOperators().Patch(ctx, OperatorConfigName, types.JSONPatchType, jsonPatchBytes, metav1.PatchOptions{}, "/status")
	return err
}
