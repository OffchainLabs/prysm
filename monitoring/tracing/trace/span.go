package trace

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// TracingEnabled tracks whether tracing is enabled in your application.
var TracingEnabled = false

// StartSpan is a wrapper over the OpenTelemetry package method. This is to allow us to skip
// calling that particular method if tracing has been disabled.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !TracingEnabled {
		// Return an empty span if tracing has been disabled.
		return ctx, noop.Span{}
	}
	tracer := otel.Tracer("")
	ctx, span := tracer.Start(ctx, name, opts...)
	return ctx, span
}

// NewContext is a wrapper which returns back the parent context
// if tracing is disabled.
func NewContext(parent context.Context, s trace.Span) context.Context {
	if !TracingEnabled {
		return parent
	}
	return trace.ContextWithSpan(parent, s)
}

// FromContext is a wrapper which returns a no-op span
// if tracing is disabled.
func FromContext(ctx context.Context) trace.Span {
	if !TracingEnabled {
		return noop.Span{}
	}
	span := trace.SpanFromContext(ctx)
	return span
}

// AddMessageSendEvent adds a message send event to the provided span.
// This function is useful for tracking the sending of messages within a trace.
func AddMessageSendEvent(span trace.Span, iid int64, uncompressedLen int64, compressedLen int64) trace.Span {
	span.AddEvent("message_send", trace.WithAttributes(
		attribute.Int64("message.id", iid),
		attribute.Int64("message.uncompressed_size", uncompressedLen),
		attribute.Int64("message.compressed_size", compressedLen),
	))
	return span
}

// Int64Attribute --
func Int64Attribute(key string, value int64) attribute.KeyValue {
	return attribute.Int64(key, value)
}

// StringAttribute --
func StringAttribute(key, value string) attribute.KeyValue {
	return attribute.String(key, value)
}

// BoolAttribute --
func BoolAttribute(key string, value bool) attribute.KeyValue {
	return attribute.Bool(key, value)
}
