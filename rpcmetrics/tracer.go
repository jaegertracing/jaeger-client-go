package rpcmetrics

import (
	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-lib/metrics"
)

// Tracer is an opentracing.Tracer decorator that can emit RPC metrics.
type Tracer struct {
	realTracer        opentracing.Tracer
	metricsByEndpoint *MetricsByEndpoint
}

// NewTracer creates a new opentracing.Tracer decorator that can emit RPC metrics.
func NewTracer(realTracer opentracing.Tracer, metricsFactory metrics.Factory) *Tracer {
	return &Tracer{
		realTracer:        realTracer,
		metricsByEndpoint: newMetricsByEndpoint(metricsFactory),
	}
}

// StartSpan implements opentracing.Tracer
func (t *Tracer) StartSpan(operationName string, options ...opentracing.StartSpanOption) opentracing.Span {
	realSpan := t.realTracer.StartSpan(operationName, options...)
	return NewSpan(t, realSpan, operationName, options...)
}

// Inject implements opentracing.Tracer
func (t *Tracer) Inject(spanCtx opentracing.SpanContext, format interface{}, carrier interface{}) error {
	return t.realTracer.Inject(spanCtx, format, carrier)
}

// Extract implements opentracing.Tracer
func (t *Tracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	return t.realTracer.Extract(format, carrier)
}
