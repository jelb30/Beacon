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
)

type linuxDetector struct {
	mode string
}

func newLinuxDetector(mode string, _ DetectorConfig) (Detector, error) {
	return &linuxDetector{mode: mode}, nil
}

func (d *linuxDetector) Name() string {
	return d.mode
}

func (d *linuxDetector) Detect(context.Context) (*Detection, error) {
	// TODO: Implement Linux cgroup PSI and eBPF/cgroup-based starvation detection.
	return nil, fmt.Errorf("linux detector mode %q is not implemented yet; use --mode synthetic for local development", d.mode)
}
