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

// Code generated by applyconfiguration-gen. DO NOT EDIT.

package v1

import (
	openshiftoperatorv1 "github.com/openshift/jobset-operator/pkg/apis/openshiftoperator/v1"
	internal "github.com/openshift/jobset-operator/pkg/generated/applyconfiguration/internal"
	apismetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	managedfields "k8s.io/apimachinery/pkg/util/managedfields"
	metav1 "k8s.io/client-go/applyconfigurations/meta/v1"
)

// JobSetOperatorApplyConfiguration represents a declarative configuration of the JobSetOperator type for use
// with apply.
type JobSetOperatorApplyConfiguration struct {
	metav1.TypeMetaApplyConfiguration    `json:",inline"`
	*metav1.ObjectMetaApplyConfiguration `json:"metadata,omitempty"`
	Spec                                 *JobSetOperatorSpecApplyConfiguration   `json:"spec,omitempty"`
	Status                               *JobSetOperatorStatusApplyConfiguration `json:"status,omitempty"`
}

// JobSetOperator constructs a declarative configuration of the JobSetOperator type for use with
// apply.
func JobSetOperator(name string) *JobSetOperatorApplyConfiguration {
	b := &JobSetOperatorApplyConfiguration{}
	b.WithName(name)
	b.WithKind("JobSetOperator")
	b.WithAPIVersion("operator.openshift.io/v1")
	return b
}

// ExtractJobSetOperator extracts the applied configuration owned by fieldManager from
// jobSetOperator. If no managedFields are found in jobSetOperator for fieldManager, a
// JobSetOperatorApplyConfiguration is returned with only the Name, Namespace (if applicable),
// APIVersion and Kind populated. It is possible that no managed fields were found for because other
// field managers have taken ownership of all the fields previously owned by fieldManager, or because
// the fieldManager never owned fields any fields.
// jobSetOperator must be a unmodified JobSetOperator API object that was retrieved from the Kubernetes API.
// ExtractJobSetOperator provides a way to perform a extract/modify-in-place/apply workflow.
// Note that an extracted apply configuration will contain fewer fields than what the fieldManager previously
// applied if another fieldManager has updated or force applied any of the previously applied fields.
// Experimental!
func ExtractJobSetOperator(jobSetOperator *openshiftoperatorv1.JobSetOperator, fieldManager string) (*JobSetOperatorApplyConfiguration, error) {
	return extractJobSetOperator(jobSetOperator, fieldManager, "")
}

// ExtractJobSetOperatorStatus is the same as ExtractJobSetOperator except
// that it extracts the status subresource applied configuration.
// Experimental!
func ExtractJobSetOperatorStatus(jobSetOperator *openshiftoperatorv1.JobSetOperator, fieldManager string) (*JobSetOperatorApplyConfiguration, error) {
	return extractJobSetOperator(jobSetOperator, fieldManager, "status")
}

func extractJobSetOperator(jobSetOperator *openshiftoperatorv1.JobSetOperator, fieldManager string, subresource string) (*JobSetOperatorApplyConfiguration, error) {
	b := &JobSetOperatorApplyConfiguration{}
	err := managedfields.ExtractInto(jobSetOperator, internal.Parser().Type("com.github.openshift.jobset-operator.pkg.apis.openshiftoperator.v1.JobSetOperator"), fieldManager, b, subresource)
	if err != nil {
		return nil, err
	}
	b.WithName(jobSetOperator.Name)

	b.WithKind("JobSetOperator")
	b.WithAPIVersion("operator.openshift.io/v1")
	return b, nil
}

// WithKind sets the Kind field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Kind field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithKind(value string) *JobSetOperatorApplyConfiguration {
	b.TypeMetaApplyConfiguration.Kind = &value
	return b
}

// WithAPIVersion sets the APIVersion field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the APIVersion field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithAPIVersion(value string) *JobSetOperatorApplyConfiguration {
	b.TypeMetaApplyConfiguration.APIVersion = &value
	return b
}

// WithName sets the Name field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Name field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithName(value string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.Name = &value
	return b
}

// WithGenerateName sets the GenerateName field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the GenerateName field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithGenerateName(value string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.GenerateName = &value
	return b
}

// WithNamespace sets the Namespace field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Namespace field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithNamespace(value string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.Namespace = &value
	return b
}

// WithUID sets the UID field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the UID field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithUID(value types.UID) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.UID = &value
	return b
}

// WithResourceVersion sets the ResourceVersion field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the ResourceVersion field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithResourceVersion(value string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.ResourceVersion = &value
	return b
}

// WithGeneration sets the Generation field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Generation field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithGeneration(value int64) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.Generation = &value
	return b
}

// WithCreationTimestamp sets the CreationTimestamp field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the CreationTimestamp field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithCreationTimestamp(value apismetav1.Time) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.CreationTimestamp = &value
	return b
}

// WithDeletionTimestamp sets the DeletionTimestamp field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the DeletionTimestamp field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithDeletionTimestamp(value apismetav1.Time) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.DeletionTimestamp = &value
	return b
}

// WithDeletionGracePeriodSeconds sets the DeletionGracePeriodSeconds field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the DeletionGracePeriodSeconds field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithDeletionGracePeriodSeconds(value int64) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	b.ObjectMetaApplyConfiguration.DeletionGracePeriodSeconds = &value
	return b
}

// WithLabels puts the entries into the Labels field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, the entries provided by each call will be put on the Labels field,
// overwriting an existing map entries in Labels field with the same key.
func (b *JobSetOperatorApplyConfiguration) WithLabels(entries map[string]string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	if b.ObjectMetaApplyConfiguration.Labels == nil && len(entries) > 0 {
		b.ObjectMetaApplyConfiguration.Labels = make(map[string]string, len(entries))
	}
	for k, v := range entries {
		b.ObjectMetaApplyConfiguration.Labels[k] = v
	}
	return b
}

// WithAnnotations puts the entries into the Annotations field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, the entries provided by each call will be put on the Annotations field,
// overwriting an existing map entries in Annotations field with the same key.
func (b *JobSetOperatorApplyConfiguration) WithAnnotations(entries map[string]string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	if b.ObjectMetaApplyConfiguration.Annotations == nil && len(entries) > 0 {
		b.ObjectMetaApplyConfiguration.Annotations = make(map[string]string, len(entries))
	}
	for k, v := range entries {
		b.ObjectMetaApplyConfiguration.Annotations[k] = v
	}
	return b
}

// WithOwnerReferences adds the given value to the OwnerReferences field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the OwnerReferences field.
func (b *JobSetOperatorApplyConfiguration) WithOwnerReferences(values ...*metav1.OwnerReferenceApplyConfiguration) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	for i := range values {
		if values[i] == nil {
			panic("nil value passed to WithOwnerReferences")
		}
		b.ObjectMetaApplyConfiguration.OwnerReferences = append(b.ObjectMetaApplyConfiguration.OwnerReferences, *values[i])
	}
	return b
}

// WithFinalizers adds the given value to the Finalizers field in the declarative configuration
// and returns the receiver, so that objects can be build by chaining "With" function invocations.
// If called multiple times, values provided by each call will be appended to the Finalizers field.
func (b *JobSetOperatorApplyConfiguration) WithFinalizers(values ...string) *JobSetOperatorApplyConfiguration {
	b.ensureObjectMetaApplyConfigurationExists()
	for i := range values {
		b.ObjectMetaApplyConfiguration.Finalizers = append(b.ObjectMetaApplyConfiguration.Finalizers, values[i])
	}
	return b
}

func (b *JobSetOperatorApplyConfiguration) ensureObjectMetaApplyConfigurationExists() {
	if b.ObjectMetaApplyConfiguration == nil {
		b.ObjectMetaApplyConfiguration = &metav1.ObjectMetaApplyConfiguration{}
	}
}

// WithSpec sets the Spec field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Spec field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithSpec(value *JobSetOperatorSpecApplyConfiguration) *JobSetOperatorApplyConfiguration {
	b.Spec = value
	return b
}

// WithStatus sets the Status field in the declarative configuration to the given value
// and returns the receiver, so that objects can be built by chaining "With" function invocations.
// If called multiple times, the Status field is set to the value of the last call.
func (b *JobSetOperatorApplyConfiguration) WithStatus(value *JobSetOperatorStatusApplyConfiguration) *JobSetOperatorApplyConfiguration {
	b.Status = value
	return b
}

// GetName retrieves the value of the Name field in the declarative configuration.
func (b *JobSetOperatorApplyConfiguration) GetName() *string {
	b.ensureObjectMetaApplyConfigurationExists()
	return b.ObjectMetaApplyConfiguration.Name
}
