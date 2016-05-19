// Copyright (c) 2015 Uber Technologies, Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package jaeger

import (
	"sync"
	"time"

	z "github.com/uber/jaeger-client-go/thrift/gen/zipkincore"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

type span struct {
	sync.RWMutex
	tracer *tracer

	TraceContext

	// The name of the "operation" this span is an instance of.
	// Known as a "span name" in some implementations.
	operationName string

	// firstInProcess, if true, indicates that this span is the root of the (sub)tree
	// of spans in the current process. In other words it's true for the root spans,
	// and the ingress spans when the process joins another trace.
	firstInProcess bool

	// used to distinguish local vs. RPC Server vs. RPC Client spans
	spanKind string

	// startTime is the timestamp indicating when the span began, with microseconds precision.
	startTime time.Time

	// duration returns duration of the span with microseconds precision.
	// Zero value means duration is unknown.
	duration time.Duration

	// peer points to the peer service participating in this span,
	// e.g. the Client if this span is a server span,
	// or Server if this span is a client span
	peer z.Endpoint

	// tags attached to this span
	tags []tag

	// The span's "micro-log"
	logs []opentracing.LogData

	// Distributed Context baggage
	baggage map[string]string
}

type tag struct {
	key   string
	value interface{}
}

// Sets or changes the operation name.
func (s *span) SetOperationName(operationName string) opentracing.Span {
	if s.IsSampled() {
		s.Lock()
		defer s.Unlock()
		s.operationName = operationName
	}
	return s
}

// SetTag implements SetTag() of opentracing.Span
func (s *span) SetTag(key string, value interface{}) opentracing.Span {
	if key == string(ext.SamplingPriority) && setSamplingPriority(s, key, value) {
		return s
	}
	if s.IsSampled() {
		handled := false
		if handler, ok := specialTagHandlers[key]; ok {
			handled = handler(s, key, value)
		}
		if !handled {
			s.Lock()
			defer s.Unlock()
			s.tags = append(s.tags, tag{key: key, value: value})
		}
	}
	return s
}

func (s *span) LogEvent(event string) {
	if s.IsSampled() {
		s.Log(opentracing.LogData{Event: event})
	}
}

func (s *span) LogEventWithPayload(event string, payload interface{}) {
	if s.IsSampled() {
		s.Log(opentracing.LogData{Event: event, Payload: payload})
	}
}

func (s *span) Log(ld opentracing.LogData) {
	if s.IsSampled() {
		s.Lock()
		defer s.Unlock()

		if ld.Timestamp.IsZero() {
			ld.Timestamp = s.tracer.timeNow()
		}
		s.logs = append(s.logs, ld)
	}
}

func (s *span) Finish() {
	s.FinishWithOptions(opentracing.FinishOptions{})
}

func (s *span) FinishWithOptions(options opentracing.FinishOptions) {
	if s.IsSampled() {
		finishTime := options.FinishTime
		if finishTime.IsZero() {
			finishTime = s.tracer.timeNow()
		}
		s.Lock()
		s.duration = finishTime.Sub(s.startTime)
		if options.BulkLogData != nil {
			s.logs = append(s.logs, options.BulkLogData...)
		}
		s.Unlock()
	}
	// call reportSpan even for non-sampled traces, to return span to the pool
	s.tracer.reportSpan(s)
}

func (s *span) SetBaggageItem(key, value string) opentracing.Span {
	key = normalizeBaggageKey(key)
	s.Lock()
	defer s.Unlock()
	if s.baggage == nil {
		s.baggage = make(map[string]string)
	}
	s.baggage[key] = value
	return s
}

func (s *span) BaggageItem(key string) string {
	key = normalizeBaggageKey(key)
	s.RLock()
	defer s.RUnlock()
	return s.baggage[key]
}

func (s *span) Tracer() opentracing.Tracer {
	return s.tracer
}

func (s *span) String() string {
	s.RLock()
	defer s.RUnlock()
	return s.TraceContext.String()
}

func (s *span) peerDefined() bool {
	return s.peer.ServiceName != "" || s.peer.Ipv4 != 0 || s.peer.Port != 0
}

func (s *span) isRPC() bool {
	s.RLock()
	defer s.RUnlock()
	return s.spanKind == string(ext.SpanKindRPCClient) || s.spanKind == string(ext.SpanKindRPCServer)
}

func (s *span) isRPCClient() bool {
	s.RLock()
	defer s.RUnlock()
	return s.spanKind == string(ext.SpanKindRPCClient)
}

var specialTagHandlers = map[string]func(*span, string, interface{}) bool{
	string(ext.SpanKind):     setSpanKind,
	string(ext.PeerHostIPv4): setPeerIPv4,
	string(ext.PeerPort):     setPeerPort,
	string(ext.PeerService):  setPeerService,
}

func setSpanKind(s *span, key string, value interface{}) bool {
	s.Lock()
	defer s.Unlock()
	if val, ok := value.(string); ok {
		s.spanKind = val
		return true
	}
	if val, ok := value.(ext.SpanKindEnum); ok {
		s.spanKind = string(val)
		return true
	}
	return false
}

func setPeerIPv4(s *span, key string, value interface{}) bool {
	s.Lock()
	defer s.Unlock()
	if val, ok := value.(string); ok {
		if ip, err := IPToUint32(val); err == nil {
			s.peer.Ipv4 = int32(ip)
			return true
		}
	}
	if val, ok := value.(uint32); ok {
		s.peer.Ipv4 = int32(val)
		return true
	}
	if val, ok := value.(int32); ok {
		s.peer.Ipv4 = val
		return true
	}
	return false
}

func setPeerPort(s *span, key string, value interface{}) bool {
	s.Lock()
	defer s.Unlock()
	if val, ok := value.(string); ok {
		if port, err := ParsePort(val); err == nil {
			s.peer.Port = int16(port)
			return true
		}
	}
	if val, ok := value.(uint16); ok {
		s.peer.Port = int16(val)
		return true
	}
	if val, ok := value.(int); ok {
		s.peer.Port = int16(val)
		return true
	}
	return false
}

func setPeerService(s *span, key string, value interface{}) bool {
	s.Lock()
	defer s.Unlock()
	if val, ok := value.(string); ok {
		s.peer.ServiceName = val
		return true
	}
	return false
}

func setSamplingPriority(s *span, key string, value interface{}) bool {
	s.Lock()
	defer s.Unlock()
	if val, ok := value.(uint16); ok {
		if val > 0 {
			s.flags = s.flags | flagDebug | flagSampled
		} else {
			s.flags = s.flags & (^flagSampled)
		}
		return true
	}
	return false
}
