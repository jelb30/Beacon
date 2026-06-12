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

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

func TestSyntheticDetectorDetect(t *testing.T) {
	detector := NewSyntheticDetector(DetectorConfig{
		Namespace:     "default",
		PolicyName:    "sample-api-policy",
		Deployment:    "sample-api",
		ContainerName: "api",
		SignalType:    autoscalingv1alpha1.StarvationSignalCPUStarvation,
		Severity:      autoscalingv1alpha1.StarvationSeverityHigh,
	})

	before := time.Now().UTC()
	detection, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if detection == nil {
		t.Fatal("Detect returned nil detection")
	}

	if detection.PolicyName != "sample-api-policy" {
		t.Fatalf("PolicyName = %q, want sample-api-policy", detection.PolicyName)
	}
	if detection.Namespace != "default" {
		t.Fatalf("Namespace = %q, want default", detection.Namespace)
	}
	if detection.TargetRef.APIVersion != "apps/v1" {
		t.Fatalf("TargetRef.APIVersion = %q, want apps/v1", detection.TargetRef.APIVersion)
	}
	if detection.TargetRef.Kind != "Deployment" {
		t.Fatalf("TargetRef.Kind = %q, want Deployment", detection.TargetRef.Kind)
	}
	if detection.TargetRef.Name != "sample-api" {
		t.Fatalf("TargetRef.Name = %q, want sample-api", detection.TargetRef.Name)
	}
	if detection.ContainerName != "api" {
		t.Fatalf("ContainerName = %q, want api", detection.ContainerName)
	}
	if detection.SignalType != autoscalingv1alpha1.StarvationSignalCPUStarvation {
		t.Fatalf("SignalType = %q, want CPUStarvation", detection.SignalType)
	}
	if detection.Severity != autoscalingv1alpha1.StarvationSeverityHigh {
		t.Fatalf("Severity = %q, want High", detection.Severity)
	}
	if detection.Source != syntheticDetectorName {
		t.Fatalf("Source = %q, want synthetic", detection.Source)
	}
	if detection.Message != "Synthetic starvation signal emitted by beacon-agent for local development." {
		t.Fatalf("Message = %q", detection.Message)
	}
	if detection.ObservedAt.Time.Before(before) {
		t.Fatalf("ObservedAt = %s, want at or after %s", detection.ObservedAt.Time, before)
	}
}
