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
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

const syntheticDetectorName = "synthetic"

// SyntheticDetector emits deterministic starvation signals for local development.
type SyntheticDetector struct {
	config DetectorConfig
}

// NewSyntheticDetector creates a detector that works on macOS, kind, and any local cluster.
func NewSyntheticDetector(config DetectorConfig) *SyntheticDetector {
	return &SyntheticDetector{config: config}
}

// Name returns the detector mode name.
func (d *SyntheticDetector) Name() string {
	return syntheticDetectorName
}

// Detect emits one synthetic starvation detection.
func (d *SyntheticDetector) Detect(ctx context.Context) (*Detection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return &Detection{
		PolicyName: d.config.PolicyName,
		Namespace:  d.config.Namespace,
		TargetRef: autoscalingv1alpha1.TargetRef{
			APIVersion: defaultAPIVersion,
			Kind:       defaultKind,
			Name:       d.config.Deployment,
		},
		ContainerName: d.config.ContainerName,
		SignalType:    d.config.SignalType,
		Severity:      d.config.Severity,
		ObservedAt:    metav1.NewTime(time.Now().UTC()),
		Message:       "Synthetic starvation signal emitted by beacon-agent for local development.",
		Source:        d.Name(),
	}, nil
}
