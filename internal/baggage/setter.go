// Copyright (c) 2017 Uber Technologies, Inc.
//
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

package baggage

import (
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"

	"github.com/uber/jaeger-client-go"
)

type spanContext interface {
	WithBaggageItem(key, value string) opentracing.SpanContext
	IsSampled() bool
	ForeachBaggageItem(handler func(k, v string) bool)
}

type span interface {
	Context() opentracing.SpanContext
	logFieldsNoLocking(fields ...log.Field)
}

// defaultBaggageSetter sets the baggage key:value on the span while respecting the
// maxValueLength and truncating the value if too long.
type DefaultBaggageSetter struct {
	restrictionManager BaggageRestrictionManager
	metrics            *jaeger.Metrics
}

func NewDefaultBaggageSetter(restrictionManager BaggageRestrictionManager, metrics *jaeger.Metrics) *DefaultBaggageSetter {
	return &DefaultBaggageSetter{
		restrictionManager: restrictionManager,
		metrics:            metrics,
	}
}

func (s *DefaultBaggageSetter) SetBaggage(sp span, key, value string) opentracing.SpanContext {
	var truncated bool
	var prevItem string
	valid, maxValueLength := s.restrictionManager.GetRestriction(key)
	if !valid {
		logFields(sp, key, value, prevItem, truncated, valid)
		s.metrics.BaggageUpdateFailure.Inc(1)
	}
	if len(value) > maxValueLength {
		truncated = true
		value = value[:maxValueLength]
		s.metrics.BaggageTruncate.Inc(1)
	}
	//prevItem = span.context.baggage[key]
	logFields(sp, key, value, prevItem, truncated, false)
	s.metrics.BaggageUpdateSuccess.Inc(1)
	return sp.Context().(spanContext).WithBaggageItem(key, value)
}

func logFields(sp span, key, value, prevItem string, truncated, valid bool) {
	if !sp.Context().(spanContext).IsSampled() {
		return
	}
	fields := []log.Field{
		log.String("event", "baggage"),
		log.String("key", key),
		log.String("value", value),
	}
	if prevItem != "" {
		fields = append(fields, log.String("override", "true"))
	}
	if truncated {
		fields = append(fields, log.String("truncated", "true"))
	}
	if !valid {
		fields = append(fields, log.String("invalid", "true"))
	}
	sp.logFieldsNoLocking(fields...)
}
