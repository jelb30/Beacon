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
	"context"
	"io"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	ServiceName        = "beacon-operator"
	tracingDisabledEnv = "BEACON_TRACING_DISABLED"
)

// InitTracing starts stdout tracing for local runs.
func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	return InitTracingWithWriter(ctx, os.Stdout)
}

// InitTracingWithWriter starts tracing with a test writer.
func InitTracingWithWriter(_ context.Context, writer io.Writer) (func(context.Context) error, error) {
	if tracingDisabled() {
		otel.SetTracerProvider(trace.NewNoopTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := stdouttrace.New(
		stdouttrace.WithWriter(writer),
		stdouttrace.WithPrettyPrint(),
	)
	if err != nil {
		return nil, err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			"",
			attribute.String("service.name", ServiceName),
		)),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

// Tracer returns a Beacon tracer.
func Tracer(instrumentationName string) trace.Tracer {
	return otel.Tracer(instrumentationName)
}

// RecordSpanError marks a span as failed when err is set.
func RecordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func tracingDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(tracingDisabledEnv)))
	return value == "1" || value == "true" || value == "yes"
}
