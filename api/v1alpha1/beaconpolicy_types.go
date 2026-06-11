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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const BeaconPolicyReadyCondition = "Ready"

// BeaconPolicyTargetRef identifies the workload controlled by a BeaconPolicy.
type BeaconPolicyTargetRef struct {
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// BeaconPolicySpec defines the desired state of BeaconPolicy
type BeaconPolicySpec struct {
	// +kubebuilder:validation:Required
	TargetRef BeaconPolicyTargetRef `json:"targetRef"`

	// +kubebuilder:validation:Required
	MinCPURequest string `json:"minCPURequest"`

	// +kubebuilder:validation:Required
	MaxCPURequest string `json:"maxCPURequest"`

	// +optional
	MinMemoryRequest string `json:"minMemoryRequest,omitempty"`

	// +optional
	MaxMemoryRequest string `json:"maxMemoryRequest,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	ScaleUpStepPercent int32 `json:"scaleUpStepPercent"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	StarvationWindowSeconds int32 `json:"starvationWindowSeconds"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	ReactionLatencyBudgetMillis int32 `json:"reactionLatencyBudgetMillis"`
}

// BeaconPolicyStatus defines the observed state of BeaconPolicy.
type BeaconPolicyStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// +optional
	LastReconcileTime *metav1.Time `json:"lastReconcileTime,omitempty"`

	// +optional
	LastDecision string `json:"lastDecision,omitempty"`

	// +optional
	LastReactionLatencyMillis int64 `json:"lastReactionLatencyMillis"`

	// +optional
	LastEventName string `json:"lastEventName,omitempty"`

	// +optional
	LastSignalType string `json:"lastSignalType,omitempty"`

	// +optional
	LastEventSeverity string `json:"lastEventSeverity,omitempty"`

	// +optional
	LastEventObservedAt *metav1.Time `json:"lastEventObservedAt,omitempty"`

	// +optional
	LastScaledAt *metav1.Time `json:"lastScaledAt,omitempty"`

	// +optional
	LastPatchedDeployment string `json:"lastPatchedDeployment,omitempty"`

	// +optional
	LastPatchedContainer string `json:"lastPatchedContainer,omitempty"`

	// +optional
	PreviousCPURequest string `json:"previousCPURequest,omitempty"`

	// +optional
	NewCPURequest string `json:"newCPURequest,omitempty"`

	// +optional
	PreviousMemoryRequest string `json:"previousMemoryRequest,omitempty"`

	// +optional
	NewMemoryRequest string `json:"newMemoryRequest,omitempty"`

	// +optional
	ScaleAction string `json:"scaleAction,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Target",type=string,JSONPath=".spec.targetRef.name"
// +kubebuilder:printcolumn:name="LastDecision",type=string,JSONPath=".status.lastDecision"
// +kubebuilder:printcolumn:name="LatencyMs",type=integer,JSONPath=".status.lastReactionLatencyMillis"
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"

// BeaconPolicy is the Schema for the beaconpolicies API
type BeaconPolicy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of BeaconPolicy
	// +required
	Spec BeaconPolicySpec `json:"spec"`

	// status defines the observed state of BeaconPolicy
	// +optional
	Status BeaconPolicyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// BeaconPolicyList contains a list of BeaconPolicy
type BeaconPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []BeaconPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BeaconPolicy{}, &BeaconPolicyList{})
}
