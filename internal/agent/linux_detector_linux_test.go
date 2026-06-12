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
	"os"
	"path/filepath"
	"testing"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

func TestParsePSI(t *testing.T) {
	reading, err := parsePSI("some avg10=12.34 avg60=2.00 avg300=0.50 total=123\nfull avg10=1.25 avg60=0.25 avg300=0.10 total=9\n")
	if err != nil {
		t.Fatalf("parsePSI returned error: %v", err)
	}

	if !reading.Some.Present {
		t.Fatal("some row was not parsed")
	}
	if reading.Some.Avg10 != 12.34 {
		t.Fatalf("some avg10 = %.2f, want 12.34", reading.Some.Avg10)
	}
	if !reading.Full.Present {
		t.Fatal("full row was not parsed")
	}
	if reading.Full.Total != 9 {
		t.Fatalf("full total = %d, want 9", reading.Full.Total)
	}
}

func TestCgroupPSIDetectorReturnsCPUDetection(t *testing.T) {
	dir := t.TempDir()
	cpuPath := filepath.Join(dir, "cpu.pressure")
	memoryPath := filepath.Join(dir, "memory.pressure")
	writePSI(t, cpuPath, "some avg10=12.50 avg60=1.00 avg300=0.10 total=100\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n")
	writePSI(t, memoryPath, "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n")

	detector := &linuxDetector{
		mode:               cgroupPSIMode,
		config:             testDetectorConfig(),
		cpuPressurePath:    cpuPath,
		memoryPressurePath: memoryPath,
		psiThresholdAvg10:  10.0,
	}

	detection, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if detection == nil {
		t.Fatal("Detect returned nil detection")
	}
	if detection.SignalType != autoscalingv1alpha1.StarvationSignalCPUStarvation {
		t.Fatalf("SignalType = %q, want CPUStarvation", detection.SignalType)
	}
	if detection.Source != cgroupPSIMode {
		t.Fatalf("Source = %q, want cgroup-psi", detection.Source)
	}
}

func TestCgroupPSIDetectorReturnsHighestPressureDetection(t *testing.T) {
	dir := t.TempDir()
	cpuPath := filepath.Join(dir, "cpu.pressure")
	memoryPath := filepath.Join(dir, "memory.pressure")
	writePSI(t, cpuPath, "some avg10=12.00 avg60=1.00 avg300=0.10 total=100\n")
	writePSI(t, memoryPath, "some avg10=20.00 avg60=1.00 avg300=0.10 total=200\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n")

	detector := &linuxDetector{
		mode:               cgroupPSIMode,
		config:             testDetectorConfig(),
		cpuPressurePath:    cpuPath,
		memoryPressurePath: memoryPath,
		psiThresholdAvg10:  10.0,
	}

	detection, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if detection == nil {
		t.Fatal("Detect returned nil detection")
	}
	if detection.SignalType != autoscalingv1alpha1.StarvationSignalMemoryStarvation {
		t.Fatalf("SignalType = %q, want MemoryStarvation", detection.SignalType)
	}
}

func TestCgroupPSIDetectorReturnsNilBelowThreshold(t *testing.T) {
	dir := t.TempDir()
	cpuPath := filepath.Join(dir, "cpu.pressure")
	memoryPath := filepath.Join(dir, "memory.pressure")
	writePSI(t, cpuPath, "some avg10=9.99 avg60=1.00 avg300=0.10 total=100\n")
	writePSI(t, memoryPath, "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\n")

	detector := &linuxDetector{
		mode:               cgroupPSIMode,
		config:             testDetectorConfig(),
		cpuPressurePath:    cpuPath,
		memoryPressurePath: memoryPath,
		psiThresholdAvg10:  10.0,
	}

	detection, err := detector.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if detection != nil {
		t.Fatalf("Detect returned detection for below-threshold pressure: %#v", detection)
	}
}

func TestCgroupPSIDetectorErrorsWhenNoPressureFilesAreReadable(t *testing.T) {
	detector := &linuxDetector{
		mode:               cgroupPSIMode,
		config:             testDetectorConfig(),
		cpuPressurePath:    "/does/not/exist/cpu.pressure",
		memoryPressurePath: "/does/not/exist/memory.pressure",
		psiThresholdAvg10:  10.0,
	}

	_, err := detector.Detect(context.Background())
	if err == nil {
		t.Fatal("Detect returned nil error")
	}
}

func testDetectorConfig() DetectorConfig {
	return DetectorConfig{
		Namespace:     "default",
		PolicyName:    "sample-api-policy",
		Deployment:    "sample-api",
		ContainerName: "api",
		Severity:      autoscalingv1alpha1.StarvationSeverityHigh,
	}
}

func writePSI(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write PSI file %s: %v", path, err)
	}
}
