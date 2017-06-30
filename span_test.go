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

type fakeBaggageRestrictionManager struct {
	restrictionsMap map[string]int
}

func (m *fakeBaggageRestrictionManager) IsValidBaggageKey(key string) (bool, int) {
	if maxValueLength, ok := m.restrictionsMap[key]; ok {
		return true, maxValueLength
	}
	return false, 0
}

func TestBaggageIterator(t *testing.T) {
	service := "DOOP"
	tracer, closer := NewTracer(service, NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*Span)
	sp1.SetBaggageItem("Some_Key", "12345")
	sp1.SetBaggageItem("Some-other-key", "42")
	expectedBaggage := map[string]string{"some-key": "12345", "some-other-key": "42"}
	assertBaggage(t, sp1, expectedBaggage)
	assertBaggageRecords(t, sp1, expectedBaggage, false)

	b := extractBaggage(sp1, false) // break out early
	assert.Equal(t, 1, len(b), "only one baggage item should be extracted")

	sp2 := tracer.StartSpan("s2", opentracing.ChildOf(sp1.Context())).(*Span)
	assertBaggage(t, sp2, expectedBaggage) // child inherits the same baggage
	require.Len(t, sp2.logs, 0)            // child doesn't inherit the baggage logs
}

func TestBaggageRestrictionManager(t *testing.T) {
	metricsFactory := metrics.NewLocalFactory(0)
	metrics := NewMetrics(metricsFactory, nil)

	service := "DOOP"
	tracer, closer := NewTracer(service, NewConstSampler(true), NewNullReporter(), TracerOptions.Metrics(metrics))
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*Span)
	sp1.baggageRestrictionManager = &fakeBaggageRestrictionManager{
		restrictionsMap: map[string]int{"some-key": 2, "some-other-key": 1},
	}
	sp1.SetBaggageItem("Some_Key", "12345")
	sp1.SetBaggageItem("Some-other-key", "42")
	sp1.SetBaggageItem("invalid-key", "moot")
	expectedBaggage := map[string]string{"some-key": "12", "some-other-key": "4"}
	assertBaggage(t, sp1, expectedBaggage)
	assertBaggageRecords(t, sp1, expectedBaggage, true)

	testutils.AssertCounterMetrics(t, metricsFactory, []testutils.ExpectedMetric{
		{Name: "jaeger.baggage-update", Tags: map[string]string{"result": "ok"}, Value: 2},
		{Name: "jaeger.baggage-update", Tags: map[string]string{"result": "err"}, Value: 1},
		{Name: "jaeger.baggage-truncate", Value: 2},
	}...)
}

func assertBaggageRecords(t *testing.T, sp *Span, expected map[string]string, truncated bool) {
	require.Len(t, sp.logs, len(expected))
	for _, logRecord := range sp.logs {
		len := 3
		if truncated {
			len = 4
		}
		require.Len(t, logRecord.Fields, len)
		require.Equal(t, "event:baggage", logRecord.Fields[0].String())
		key := logRecord.Fields[1].Value().(string)
		value := logRecord.Fields[2].Value().(string)

		require.Contains(t, expected, key)
		assert.Equal(t, expected[key], value)
		if truncated {
			assert.Equal(t, "true", logRecord.Fields[3].Value().(string))
		}
	}
}

func assertBaggage(t *testing.T, sp opentracing.Span, expected map[string]string) {
	b := extractBaggage(sp, true)
	assert.Equal(t, expected, b)
}

func extractBaggage(sp opentracing.Span, allItems bool) map[string]string {
	b := make(map[string]string)
	sp.Context().ForeachBaggageItem(func(k, v string) bool {
		b[k] = v
		return allItems
	})
	return b
}

func TestSpanProperties(t *testing.T) {
	tracer, closer := NewTracer("DOOP", NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*Span)
	assert.Equal(t, tracer, sp1.Tracer())
	assert.NotNil(t, sp1.Context())
}

func TestSpanOperationName(t *testing.T) {
	tracer, closer := NewTracer("DOOP", NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*Span)
	sp1.SetOperationName("s2")
	sp1.Finish()

	assert.Equal(t, "s2", sp1.OperationName())
}
