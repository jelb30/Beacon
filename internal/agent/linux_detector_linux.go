//go:build linux

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
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

const (
	cgroupPSIMode = "cgroup-psi"
	ebpfMode      = "ebpf"

	defaultCgroupPSIThresholdAvg10 = 10.0
	defaultCPUPressurePath         = "/sys/fs/cgroup/cpu.pressure"
	defaultMemoryPressurePath      = "/sys/fs/cgroup/memory.pressure"

	envCgroupPSIThresholdAvg10 = "BEACON_CGROUP_PSI_THRESHOLD_AVG10"
	envCPUPressurePath         = "BEACON_CGROUP_PSI_CPU_PATH"
	envMemoryPressurePath      = "BEACON_CGROUP_PSI_MEMORY_PATH"
)

type linuxDetector struct {
	mode               string
	config             DetectorConfig
	cpuPressurePath    string
	memoryPressurePath string
	psiThresholdAvg10  float64
}

type psiLine struct {
	Present bool
	Avg10   float64
	Avg60   float64
	Avg300  float64
	Total   uint64
}

type psiReading struct {
	Resource   string
	SignalType string
	Path       string
	Some       psiLine
	Full       psiLine
}

type psiCandidate struct {
	Reading psiReading
	Metric  string
	Value   float64
}

func newLinuxDetector(mode string, config DetectorConfig) (Detector, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = cgroupPSIMode
	}

	switch mode {
	case cgroupPSIMode:
		return &linuxDetector{
			mode:               cgroupPSIMode,
			config:             config,
			cpuPressurePath:    envString(envCPUPressurePath, defaultCPUPressurePath),
			memoryPressurePath: envString(envMemoryPressurePath, defaultMemoryPressurePath),
			psiThresholdAvg10:  envFloat(envCgroupPSIThresholdAvg10, defaultCgroupPSIThresholdAvg10),
		}, nil
	case ebpfMode:
		return &linuxDetector{mode: ebpfMode, config: config}, nil
	default:
		return nil, fmt.Errorf("unsupported linux detector mode %q", mode)
	}
}

func (d *linuxDetector) Name() string {
	return d.mode
}

func (d *linuxDetector) Detect(ctx context.Context) (*Detection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	switch d.mode {
	case cgroupPSIMode:
		return d.detectCgroupPSI()
	case ebpfMode:
		return nil, fmt.Errorf("linux detector mode %q is not implemented yet; use --mode cgroup-psi or --mode synthetic", d.mode)
	default:
		return nil, fmt.Errorf("unsupported linux detector mode %q", d.mode)
	}
}

func (d *linuxDetector) detectCgroupPSI() (*Detection, error) {
	readings, err := d.readPressureFiles()
	if err != nil {
		return nil, err
	}

	candidate, ok := highestPressureCandidate(readings, d.psiThresholdAvg10)
	if !ok {
		return nil, nil
	}

	now := metav1.NewTime(time.Now().UTC())
	return &Detection{
		PolicyName: candidateConfigValue(d.config.PolicyName, "sample-api-policy"),
		Namespace:  candidateConfigValue(d.config.Namespace, "default"),
		TargetRef: autoscalingv1alpha1.TargetRef{
			APIVersion: defaultAPIVersion,
			Kind:       defaultKind,
			Name:       candidateConfigValue(d.config.Deployment, "sample-api"),
		},
		ContainerName: candidateConfigValue(d.config.ContainerName, "api"),
		SignalType:    candidate.Reading.SignalType,
		Severity:      candidateConfigValue(d.config.Severity, autoscalingv1alpha1.StarvationSeverityHigh),
		ObservedAt:    now,
		Message:       cgroupPSIMessage(candidate, d.psiThresholdAvg10),
		Source:        cgroupPSIMode,
	}, nil
}

func (d *linuxDetector) readPressureFiles() ([]psiReading, error) {
	candidates := []struct {
		resource   string
		signalType string
		path       string
	}{
		{
			resource:   "cpu",
			signalType: autoscalingv1alpha1.StarvationSignalCPUStarvation,
			path:       d.cpuPressurePath,
		},
		{
			resource:   "memory",
			signalType: autoscalingv1alpha1.StarvationSignalMemoryStarvation,
			path:       d.memoryPressurePath,
		},
	}

	var readings []psiReading
	var errors []string
	for _, candidate := range candidates {
		reading, err := readPSIFile(candidate.resource, candidate.signalType, candidate.path)
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		readings = append(readings, reading)
	}

	if len(readings) == 0 {
		return nil, fmt.Errorf("unable to read cgroup PSI files: %s", strings.Join(errors, "; "))
	}

	return readings, nil
}

func readPSIFile(resource string, signalType string, path string) (psiReading, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return psiReading{}, fmt.Errorf("read %s PSI file %s: %w", resource, path, err)
	}

	reading, err := parsePSI(string(content))
	if err != nil {
		return psiReading{}, fmt.Errorf("parse %s PSI file %s: %w", resource, path, err)
	}
	reading.Resource = resource
	reading.SignalType = signalType
	reading.Path = path

	return reading, nil
}

func parsePSI(content string) (psiReading, error) {
	var reading psiReading

	for _, rawLine := range strings.Split(content, "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			continue
		}

		fields := strings.Fields(rawLine)
		if len(fields) < 2 {
			return psiReading{}, fmt.Errorf("invalid PSI line %q", rawLine)
		}

		line, err := parsePSILine(fields[1:])
		if err != nil {
			return psiReading{}, fmt.Errorf("invalid PSI line %q: %w", rawLine, err)
		}

		switch fields[0] {
		case "some":
			line.Present = true
			reading.Some = line
		case "full":
			line.Present = true
			reading.Full = line
		default:
			return psiReading{}, fmt.Errorf("unknown PSI pressure type %q", fields[0])
		}
	}

	if !reading.Some.Present && !reading.Full.Present {
		return psiReading{}, fmt.Errorf("missing some/full PSI rows")
	}

	return reading, nil
}

func parsePSILine(fields []string) (psiLine, error) {
	var line psiLine
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return psiLine{}, fmt.Errorf("missing key/value in %q", field)
		}

		switch key {
		case "avg10":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return psiLine{}, fmt.Errorf("parse avg10: %w", err)
			}
			line.Avg10 = parsed
		case "avg60":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return psiLine{}, fmt.Errorf("parse avg60: %w", err)
			}
			line.Avg60 = parsed
		case "avg300":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return psiLine{}, fmt.Errorf("parse avg300: %w", err)
			}
			line.Avg300 = parsed
		case "total":
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return psiLine{}, fmt.Errorf("parse total: %w", err)
			}
			line.Total = parsed
		}
	}

	return line, nil
}

func highestPressureCandidate(readings []psiReading, threshold float64) (psiCandidate, bool) {
	var selected psiCandidate
	for _, reading := range readings {
		metric, value, ok := reading.maxAvg10()
		if !ok || value <= threshold {
			continue
		}
		if selected.Reading.Resource == "" || value > selected.Value {
			selected = psiCandidate{
				Reading: reading,
				Metric:  metric,
				Value:   value,
			}
		}
	}

	return selected, selected.Reading.Resource != ""
}

func (r psiReading) maxAvg10() (string, float64, bool) {
	if r.Some.Present && (!r.Full.Present || r.Some.Avg10 >= r.Full.Avg10) {
		return "some", r.Some.Avg10, true
	}
	if r.Full.Present {
		return "full", r.Full.Avg10, true
	}

	return "", 0, false
}

func cgroupPSIMessage(candidate psiCandidate, threshold float64) string {
	return fmt.Sprintf(
		"cgroup PSI %s pressure crossed avg10 threshold: %s avg10=%.2f threshold=%.2f some_avg10=%s full_avg10=%s path=%s",
		candidate.Reading.Resource,
		candidate.Metric,
		candidate.Value,
		threshold,
		formatOptionalAvg10(candidate.Reading.Some),
		formatOptionalAvg10(candidate.Reading.Full),
		candidate.Reading.Path,
	)
}

func formatOptionalAvg10(line psiLine) string {
	if !line.Present {
		return "n/a"
	}

	return fmt.Sprintf("%.2f", line.Avg10)
}

func candidateConfigValue(value string, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}

	return value
}

func envString(name string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	return value
}

func envFloat(name string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}

	return parsed
}
