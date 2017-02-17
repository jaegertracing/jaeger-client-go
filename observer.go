package jaeger

import opentracing "github.com/opentracing/opentracing-go"

// Observer can be registered with the Tracer to receive notifications about new Spans.
type Observer interface {
	OnStartSpan(operationName string, options opentracing.StartSpanOptions) SpanObserver
}

// SpanObserver is created by the Observer and receives notifications about other Span events.
type SpanObserver interface {
	OnSetOperationName(operationName string)
	OnSetTag(key string, value interface{})
	OnFinish(options opentracing.FinishOptions)
}
