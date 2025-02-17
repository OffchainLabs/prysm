// Package tracing sets up jaeger as an opentracing tool
// for services in Prysm.
package tracing

import (
	"context"
	"errors"
	"time"

	prysmTrace "github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace/noop"
)

var log = logrus.WithField("prefix", "tracing")

// Setup creates and initializes a new Jaegar tracing configuration with opentelemetry.
func Setup(ctx context.Context, serviceName, processName, endpoint string, sampleFraction float64, enable bool) error {
	if !enable {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return nil
	}
	prysmTrace.TracingEnabled = true

	if serviceName == "" {
		return errors.New("tracing service name cannot be empty")
	}

	log.Infof("Starting otel exporter endpoint at address = %s", endpoint)
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpoint))
	if err != nil {
		return err
	}
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.TraceIDRatioBased(sampleFraction)),
		trace.WithBatcher(
			exporter,
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
			trace.WithBatchTimeout(trace.DefaultScheduleDelay*time.Millisecond),
			trace.WithMaxExportBatchSize(trace.DefaultMaxExportBatchSize),
		),
		trace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(serviceName),
				attribute.String("process_name", processName),
			),
		),
	)

	otel.SetTracerProvider(tp)
	return nil
}
