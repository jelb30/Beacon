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

package observability

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const unknownMetricLabel = "unknown"

var (
	registerMetricsOnce sync.Once
	registerMetricsErr  error

	reconcileLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "beacon",
			Name:      "reconcile_latency_milliseconds",
			Help:      "Beacon reconcile latency in milliseconds.",
			Buckets:   []float64{10, 50, 100, 250, 500, 1000, 2000, 4000, 8000, 16000},
		},
		[]string{"controller", "signal_type", "severity", "scale_action"},
	)

	starvationEventsProcessed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "beacon",
			Name:      "starvation_events_processed_total",
			Help:      "Total StarvationEvent resources processed by Beacon.",
		},
		[]string{"signal_type", "severity", "scale_action"},
	)

	verticalScalePatches = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "beacon",
			Name:      "vertical_scale_patches_total",
			Help:      "Total Deployment resource request patches applied by Beacon.",
		},
		[]string{"signal_type", "severity", "scale_action"},
	)

	verticalScaleErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "beacon",
			Name:      "vertical_scale_errors_total",
			Help:      "Total vertical scaling errors observed by Beacon.",
		},
		[]string{"signal_type", "severity", "scale_action"},
	)
)

// RegisterMetrics adds Beacon metrics to the manager registry.
func RegisterMetrics() error {
	registerMetricsOnce.Do(func() {
		registerMetricsErr = registerCollectors(
			reconcileLatency,
			starvationEventsProcessed,
			verticalScalePatches,
			verticalScaleErrors,
		)
	})

	return registerMetricsErr
}

func registerCollectors(collectors ...prometheus.Collector) error {
	for _, collector := range collectors {
		if err := metrics.Registry.Register(collector); err != nil {
			return err
		}
	}

	return nil
}

// RecordReconcileLatency records one reconcile duration.
func RecordReconcileLatency(controller string, signalType string, severity string, scaleAction string, duration time.Duration) {
	reconcileLatency.WithLabelValues(
		metricLabel(controller),
		metricLabel(signalType),
		metricLabel(severity),
		metricLabel(scaleAction),
	).Observe(float64(duration.Milliseconds()))
}

// RecordStarvationEventProcessed counts a processed starvation event.
func RecordStarvationEventProcessed(signalType string, severity string, scaleAction string) {
	starvationEventsProcessed.WithLabelValues(
		metricLabel(signalType),
		metricLabel(severity),
		metricLabel(scaleAction),
	).Inc()
}

// RecordVerticalScalePatch counts a Deployment patch.
func RecordVerticalScalePatch(signalType string, severity string, scaleAction string) {
	verticalScalePatches.WithLabelValues(
		metricLabel(signalType),
		metricLabel(severity),
		metricLabel(scaleAction),
	).Inc()
}

// RecordVerticalScaleError counts a scaling error.
func RecordVerticalScaleError(signalType string, severity string, scaleAction string) {
	verticalScaleErrors.WithLabelValues(
		metricLabel(signalType),
		metricLabel(severity),
		metricLabel(scaleAction),
	).Inc()
}

func metricLabel(value string) string {
	if value == "" {
		return unknownMetricLabel
	}

	return value
}
