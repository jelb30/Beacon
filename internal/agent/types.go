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
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

const (
	appNameLabel      = "app.kubernetes.io/name"
	componentLabel    = "app.kubernetes.io/component"
	agentSourceLabel  = "beacon.dev/source"
	agentComponent    = "agent"
	agentAppName      = "beacon"
	defaultAPIVersion = "apps/v1"
	defaultKind       = "Deployment"
)

// Detection is one starvation signal from a detector.
type Detection struct {
	PolicyName    string
	Namespace     string
	TargetRef     autoscalingv1alpha1.TargetRef
	ContainerName string
	SignalType    string
	Severity      string
	ObservedAt    metav1.Time
	Message       string
	Source        string
}

// Detector reads a signal source and returns one detection.
type Detector interface {
	Detect(ctx context.Context) (*Detection, error)
	Name() string
}

// DetectorConfig holds the shared settings for detector modes.
type DetectorConfig struct {
	Namespace     string
	PolicyName    string
	Deployment    string
	ContainerName string
	SignalType    string
	Severity      string
}

// NewDetector returns the detector for the selected mode.
func NewDetector(mode string, config DetectorConfig) (Detector, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "synthetic":
		return NewSyntheticDetector(config), nil
	case "cgroup-psi", "ebpf":
		return newLinuxDetector(mode, config)
	default:
		return nil, fmt.Errorf("unsupported detector mode %q", mode)
	}
}

// CreateStarvationEvent writes a detection as a StarvationEvent.
func CreateStarvationEvent(ctx context.Context, c client.Client, detection Detection) error {
	event := starvationEventForDetection(detection)
	if err := c.Create(ctx, event); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}

		event.Name = starvationEventName(detection)
		if retryErr := c.Create(ctx, event); retryErr != nil {
			return retryErr
		}
	}

	return nil
}

func starvationEventForDetection(detection Detection) *autoscalingv1alpha1.StarvationEvent {
	source := detection.Source
	if source == "" {
		source = "unknown"
	}

	return &autoscalingv1alpha1.StarvationEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      starvationEventName(detection),
			Namespace: detection.Namespace,
			Labels: map[string]string{
				appNameLabel:     agentAppName,
				componentLabel:   agentComponent,
				agentSourceLabel: labelValue(source),
			},
		},
		Spec: autoscalingv1alpha1.StarvationEventSpec{
			PolicyName:    detection.PolicyName,
			TargetRef:     detection.TargetRef,
			ContainerName: detection.ContainerName,
			SignalType:    detection.SignalType,
			ObservedAt:    detection.ObservedAt,
			Severity:      detection.Severity,
			Message:       detection.Message,
		},
	}
}

func starvationEventName(detection Detection) string {
	signal := namePart(detection.SignalType)
	if signal == "" {
		signal = "starvation"
	}

	return fmt.Sprintf("beacon-agent-%s-%d-%s", signal, detection.ObservedAt.Unix(), randomSuffix())
}

func randomSuffix() string {
	bytes := make([]byte, 3)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}

	return hex.EncodeToString(bytes)
}

func namePart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastHyphen := false

	for _, r := range value {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if valid {
			builder.WriteRune(r)
			lastHyphen = false
			continue
		}
		if !lastHyphen {
			builder.WriteByte('-')
			lastHyphen = true
		}
	}

	return strings.Trim(builder.String(), "-")
}

func labelValue(value string) string {
	slug := namePart(value)
	if slug == "" {
		return "unknown"
	}
	if len(slug) > 63 {
		return strings.Trim(slug[:63], "-")
	}

	return slug
}
