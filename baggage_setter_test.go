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

package jaeger

import (
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"
)

func withTracerAndMetrics(f func(tracer *Tracer, metrics *Metrics, factory *metrics.LocalFactory)) {
	factory := metrics.NewLocalFactory(0)
	m := NewMetrics(factory, nil)

	service := "DOOP"
	tracer, closer := NewTracer(service, NewConstSampler(true), NewNullReporter())
	defer closer.Close()
	f(tracer.(*Tracer), m, factory)
}

func TestTruncateBaggage(t *testing.T) {
	withTracerAndMetrics(func(tracer *Tracer, metrics *Metrics, factory *metrics.LocalFactory) {
		setter := NewBaggageSetter(true, 5, metrics)
		key := "key"
		value := "01234567890"
		expected := "01234"

		parent := tracer.StartSpan("parent").(*Span)
		parent.context = parent.context.WithBaggageItem(key, value)
		span := tracer.StartSpan("child", opentracing.ChildOf(parent.Context())).(*Span)

		ctx := setter.setBaggage(span, key, value)
		assertBaggageFields(t, span, key, expected, true, true, false)
		assert.Equal(t, expected, ctx.baggage[key])

		testutils.AssertCounterMetrics(t, factory,
			testutils.ExpectedMetric{
				Name:  "jaeger.baggage-truncate",
				Value: 1,
			},
			testutils.ExpectedMetric{
				Name:  "jaeger.baggage-update",
				Tags:  map[string]string{"result": "ok"},
				Value: 1,
			},
		)
	})
}

func TestInvalidBaggage(t *testing.T) {
	withTracerAndMetrics(func(tracer *Tracer, metrics *Metrics, factory *metrics.LocalFactory) {
		setter := NewBaggageSetter(false, 0, metrics)
		key := "key"
		value := "value"

		span := tracer.StartSpan("span").(*Span)

		ctx := setter.setBaggage(span, key, value)
		assertBaggageFields(t, span, key, value, false, false, true)
		assert.Empty(t, ctx.baggage[key])

		testutils.AssertCounterMetrics(t, factory,
			testutils.ExpectedMetric{
				Name:  "jaeger.baggage-update",
				Tags:  map[string]string{"result": "err"},
				Value: 1,
			},
		)
	})
}

func assertBaggageFields(t *testing.T, sp *Span, key, value string, override, truncated, invalid bool) {
	require.Len(t, sp.logs, 1)
	keys := map[string]struct{}{}
	for _, field := range sp.logs[0].Fields {
		keys[field.String()] = struct{}{}
	}
	assert.Contains(t, keys, "event:baggage")
	assert.Contains(t, keys, "key:"+key)
	assert.Contains(t, keys, "value:"+value)
	if invalid {
		assert.Contains(t, keys, "invalid:true")
	}
	if override {
		assert.Contains(t, keys, "override:true")
	}
	if truncated {
		assert.Contains(t, keys, "truncated:true")
	}
}
