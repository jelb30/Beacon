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

package agent

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

func TestCreateStarvationEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := autoscalingv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme returned error: %v", err)
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	observedAt := metav1.NewTime(time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC))
	detection := Detection{
		PolicyName: "sample-api-policy",
		Namespace:  "default",
		TargetRef: autoscalingv1alpha1.TargetRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       "sample-api",
		},
		ContainerName: "api",
		SignalType:    autoscalingv1alpha1.StarvationSignalCPUStarvation,
		Severity:      autoscalingv1alpha1.StarvationSeverityHigh,
		ObservedAt:    observedAt,
		Message:       "test detection",
		Source:        "synthetic",
	}

	if err := CreateStarvationEvent(context.Background(), k8sClient, detection); err != nil {
		t.Fatalf("CreateStarvationEvent returned error: %v", err)
	}

	events := &autoscalingv1alpha1.StarvationEventList{}
	if err := k8sClient.List(context.Background(), events, client.InNamespace("default")); err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(events.Items) != 1 {
		t.Fatalf("created events = %d, want 1", len(events.Items))
	}

	event := events.Items[0]
	if event.Name == "" {
		t.Fatal("event name is empty")
	}
	if event.Labels[appNameLabel] != agentAppName {
		t.Fatalf("app label = %q, want %q", event.Labels[appNameLabel], agentAppName)
	}
	if event.Labels[componentLabel] != agentComponent {
		t.Fatalf("component label = %q, want %q", event.Labels[componentLabel], agentComponent)
	}
	if event.Labels[agentSourceLabel] != "synthetic" {
		t.Fatalf("source label = %q, want synthetic", event.Labels[agentSourceLabel])
	}
	if event.Spec.PolicyName != detection.PolicyName {
		t.Fatalf("PolicyName = %q, want %q", event.Spec.PolicyName, detection.PolicyName)
	}
	if event.Spec.TargetRef != detection.TargetRef {
		t.Fatalf("TargetRef = %#v, want %#v", event.Spec.TargetRef, detection.TargetRef)
	}
	if event.Spec.ContainerName != detection.ContainerName {
		t.Fatalf("ContainerName = %q, want %q", event.Spec.ContainerName, detection.ContainerName)
	}
	if event.Spec.SignalType != detection.SignalType {
		t.Fatalf("SignalType = %q, want %q", event.Spec.SignalType, detection.SignalType)
	}
	if event.Spec.Severity != detection.Severity {
		t.Fatalf("Severity = %q, want %q", event.Spec.Severity, detection.Severity)
	}
	if !event.Spec.ObservedAt.Equal(&observedAt) {
		t.Fatalf("ObservedAt = %s, want %s", event.Spec.ObservedAt.Time, observedAt.Time)
	}
}
