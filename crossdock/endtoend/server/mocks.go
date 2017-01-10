package server

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

type mockSpan struct {}

func (t *mockSpan) Finish() {}

func (t *mockSpan) FinishWithOptions(opts opentracing.FinishOptions)() {}

func (t *mockSpan) Context() opentracing.SpanContext { return nil }

func (t *mockSpan) SetOperationName(operationName string) opentracing.Span { return nil }

func (t *mockSpan) SetTag(key string, value interface{}) opentracing.Span { return nil }

func (t *mockSpan) LogFields(fields ...log.Field) {}

func (t *mockSpan) LogKV(alternatingKeyValues ...interface{}) {}

func (t *mockSpan) SetBaggageItem(restrictedKey, value string) opentracing.Span { return nil }

func (t *mockSpan) BaggageItem(restrictedKey string) string { return "" }

func (t *mockSpan) Tracer() opentracing.Tracer { return nil }

func (t *mockSpan) LogEvent(event string) {}

func (t *mockSpan) LogEventWithPayload(event string, payload interface{}) {}

func (t *mockSpan) Log(data opentracing.LogData) {}

type mockTracer struct {
	count map[string]int
}

func newMockTracer() *mockTracer {
	return &mockTracer{count: map[string]int{}}
}

func (t *mockTracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	t.count[operationName]++
	return &mockSpan{}
}

func (t *mockTracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) error {
	return nil
}

func (t *mockTracer) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	return nil, nil
}