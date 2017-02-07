package rpcmetrics

import (
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
)

// Span is a wrapper around opentracing.Span that collects RPC metrics
type Span struct {
	tracer        *Tracer
	realSpan      opentracing.Span
	operationName string
	startTime     time.Time
	mux           sync.Mutex
}

// NewSpan creates a new opentracing.Span decorator that can emit RPC metrics.
func NewSpan(
	tracer *Tracer,
	realSpan opentracing.Span,
	operationName string,
	opts ...opentracing.StartSpanOption,
) *Span {
	options := opentracing.StartSpanOptions{}
	for _, opt := range opts {
		opt.Apply(&options)
	}
	if options.StartTime.IsZero() {
		options.StartTime = time.Now()
	}
	s := &Span{
		tracer:        tracer,
		realSpan:      realSpan,
		operationName: operationName,
		startTime:     options.StartTime,
	}
	for k, v := range options.Tags {
		s.handleTag(k, v)
	}
	return s
}

// handleTags watches for special tags
// - SpanKind
// - HttpStatusCode
func (s *Span) handleTag(key string, value interface{}) {

}

// Finish implements opentracing.Span
func (s *Span) Finish() {
	s.FinishWithOptions(opentracing.FinishOptions{})
}

// FinishWithOptions implements opentracing.Span
func (s *Span) FinishWithOptions(opts opentracing.FinishOptions) {
	s.realSpan.FinishWithOptions(opts)
	// TODO emit metrics
}

// Context implements opentracing.Span
func (s *Span) Context() opentracing.SpanContext {
	return s.realSpan.Context()
}

// SetOperationName implements opentracing.Span
func (s *Span) SetOperationName(operationName string) opentracing.Span {
	s.mux.Lock()
	s.operationName = operationName
	s.mux.Unlock()
	s.realSpan.SetOperationName(operationName)
	return s
}

// SetTag implements opentracing.Span
func (s *Span) SetTag(key string, value interface{}) opentracing.Span {
	s.realSpan.SetTag(key, value)
	// TODO check for RPC tags
	return s
}

// LogFields implements opentracing.Span
func (s *Span) LogFields(fields ...log.Field) {
	s.realSpan.LogFields(fields...)
}

// LogKV implements opentracing.Span
func (s *Span) LogKV(alternatingKeyValues ...interface{}) {
	s.realSpan.LogKV(alternatingKeyValues...)
}

// SetBaggageItem implements opentracing.Span
func (s *Span) SetBaggageItem(key, value string) opentracing.Span {
	s.realSpan.SetBaggageItem(key, value)
	return s
}

// BaggageItem implements opentracing.Span
func (s *Span) BaggageItem(key string) string {
	return s.realSpan.BaggageItem(key)
}

// Tracer implements opentracing.Span
func (s *Span) Tracer() opentracing.Tracer {
	return s.tracer
}

// LogEvent implements opentracing.Span
func (s *Span) LogEvent(event string) {
	s.realSpan.LogEvent(event)
}

// LogEventWithPayload implements opentracing.Span
func (s *Span) LogEventWithPayload(event string, payload interface{}) {
	s.realSpan.LogEventWithPayload(event, payload)
}

// Log implements opentracing.Span
func (s *Span) Log(data opentracing.LogData) {
	s.realSpan.Log(data)
}
