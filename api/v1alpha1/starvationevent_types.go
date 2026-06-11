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

const (
	StarvationEventProcessedCondition = "Processed"

	StarvationSignalCPUStarvation    = "CPUStarvation"
	StarvationSignalMemoryStarvation = "MemoryStarvation"

	StarvationSeverityLow      = "Low"
	StarvationSeverityMedium   = "Medium"
	StarvationSeverityHigh     = "High"
	StarvationSeverityCritical = "Critical"
)

// StarvationEventTargetRef identifies the workload associated with a starvation signal.
type StarvationEventTargetRef struct {
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// StarvationEventSpec defines the desired state of StarvationEvent
type StarvationEventSpec struct {
	// +kubebuilder:validation:Required
	PolicyName string `json:"policyName"`

	// +kubebuilder:validation:Required
	TargetRef StarvationEventTargetRef `json:"targetRef"`

	// +kubebuilder:validation:Required
	ContainerName string `json:"containerName"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=CPUStarvation;MemoryStarvation
	SignalType string `json:"signalType"`

	// +kubebuilder:validation:Required
	ObservedAt metav1.Time `json:"observedAt"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Low;Medium;High;Critical
	Severity string `json:"severity"`

	// +optional
	Message string `json:"message,omitempty"`
}

// StarvationEventStatus defines the observed state of StarvationEvent.
type StarvationEventStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// +optional
	Processed bool `json:"processed"`

	// +optional
	ProcessedAt *metav1.Time `json:"processedAt,omitempty"`

	// +optional
	RoutedPolicy string `json:"routedPolicy,omitempty"`

	// +optional
	ReactionLatencyMillis int64 `json:"reactionLatencyMillis"`

	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Policy",type=string,JSONPath=".spec.policyName"
// +kubebuilder:printcolumn:name="Signal",type=string,JSONPath=".spec.signalType"
// +kubebuilder:printcolumn:name="Severity",type=string,JSONPath=".spec.severity"
// +kubebuilder:printcolumn:name="Processed",type=boolean,JSONPath=".status.processed"
// +kubebuilder:printcolumn:name="LatencyMs",type=integer,JSONPath=".status.reactionLatencyMillis"

// StarvationEvent is the Schema for the starvationevents API
type StarvationEvent struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of StarvationEvent
	// +required
	Spec StarvationEventSpec `json:"spec"`

	// status defines the observed state of StarvationEvent
	// +optional
	Status StarvationEventStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// StarvationEventList contains a list of StarvationEvent
type StarvationEventList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []StarvationEvent `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StarvationEvent{}, &StarvationEventList{})
}
