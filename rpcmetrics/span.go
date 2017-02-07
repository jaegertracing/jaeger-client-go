package rpcmetrics

import (
	"sync"
	"time"

	"strconv"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/opentracing/opentracing-go/log"
)

// SpanKind identifies the span as inboud, outbound, or internal
type SpanKind int

const (
	// Local span kind
	Local SpanKind = iota
	// Inbound span kind
	Inbound
	// Outbound span kind
	Outbound
)

// Span is a wrapper around opentracing.Span that collects RPC metrics
type Span struct {
	tracer         *Tracer
	realSpan       opentracing.Span
	operationName  string
	startTime      time.Time
	mux            sync.Mutex
	kind           SpanKind
	httpStatusCode uint16
	err            bool
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
		s.handleTagInLock(k, v)
	}
	return s
}

// handleTags watches for special tags
// - SpanKind
// - HttpStatusCode
// - Error
func (s *Span) handleTagInLock(key string, value interface{}) {
	if key == string(ext.SpanKind) {
		if v, ok := value.(string); ok {
			if v == string(ext.SpanKindRPCClientEnum) {
				s.kind = Outbound
			} else if v == string(ext.SpanKindRPCServerEnum) {
				s.kind = Inbound
			}
		}
		return
	}
	if key == string(ext.HTTPStatusCode) {
		if v, ok := value.(uint16); ok {
			s.httpStatusCode = v
		} else if v, ok := value.(string); ok {
			if vv, err := strconv.Atoi(v); err == nil {
				s.httpStatusCode = uint16(vv)
			}
		}
		return
	}
	if key == string(ext.Error) {
		if v, ok := value.(bool); ok {
			s.err = v
		} else if v, ok := value.(string); ok {
			if vv, err := strconv.ParseBool(v); err == nil {
				s.err = vv
			}
		}
		return
	}
}

// Finish implements opentracing.Span
func (s *Span) Finish() {
	s.FinishWithOptions(opentracing.FinishOptions{})
}

// FinishWithOptions implements opentracing.Span
func (s *Span) FinishWithOptions(opts opentracing.FinishOptions) {
	s.realSpan.FinishWithOptions(opts)

	s.mux.Lock()
	defer s.mux.Unlock()

	// emit metrics

	if s.operationName == "" || s.kind != Inbound {
		return
	}

	endTime := opts.FinishTime
	if endTime.IsZero() {
		endTime = time.Now()
	}

	mets := s.tracer.metricsByEndpoint.get(s.operationName)
	mets.Requests.Inc(1)
	if s.err {
		mets.Failures.Inc(1)
	} else {
		mets.Success.Inc(1)
	}
	mets.RequestLatencyMs.Record(endTime.Sub(s.startTime))
	mets.recordHTTPStatusCode(s.httpStatusCode)
}

// Context implements opentracing.Span
func (s *Span) Context() opentracing.SpanContext {
	return s.realSpan.Context()
}

// SetOperationName implements opentracing.Span
func (s *Span) SetOperationName(operationName string) opentracing.Span {
	s.realSpan.SetOperationName(operationName)
	s.mux.Lock()
	s.operationName = operationName
	s.mux.Unlock()
	return s
}

// SetTag implements opentracing.Span
func (s *Span) SetTag(key string, value interface{}) opentracing.Span {
	s.realSpan.SetTag(key, value)
	s.mux.Lock()
	s.handleTagInLock(key, value)
	s.mux.Unlock()
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
