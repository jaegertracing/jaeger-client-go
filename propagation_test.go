package jaeger

import (
	"bytes"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanPropagator(t *testing.T) {
	var err error
	const op = "test"
	reporter := NewInMemoryReporter()
	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)
	tracer, closer := NewTracer("x", NewConstSampler(true), reporter, TracerOptions.Metrics(metrics))

	tmc := opentracing.HTTPHeaderTextMapCarrier(http.Header{})
	tests := []struct {
		format, carrier interface{}
	}{
		{TraceContextFormat, &TraceContextCarrier{}},
		{opentracing.Binary, &bytes.Buffer{}},
		{opentracing.TextMap, tmc},
	}

	sp := tracer.StartSpan(op)
	sp.SetBaggageItem("foo", "bar")
	for i, test := range tests {
		child := opentracing.StartChildSpan(sp, op)
		if err := tracer.Inject(child, test.format, test.carrier); err != nil {
			t.Fatalf("test %d, format %+v: %v", i, test.format, err)
		}
		// Note: we're not finishing the above span
		child, err = tracer.Join(op, test.format, test.carrier)
		if err != nil {
			t.Fatalf("test %d, format %+v: %v", i, test.format, err)
		}
		child.Finish()
	}
	sp.Finish()
	closer.Close()

	otSpans := reporter.GetSpans()
	if a, e := len(otSpans), len(tests)+1; a != e {
		t.Fatalf("expected %d spans, got %d", e, a)
	}

	spans := make([]*span, len(otSpans))
	for i, s := range otSpans {
		spans[i] = s.(*span)
	}

	// The last span is the original one.
	exp, spans := spans[len(spans)-1], spans[:len(spans)-1]
	exp.duration = time.Duration(123)
	exp.startTime = time.Time{}.Add(1)

	if exp.ParentID() != 0 {
		t.Fatalf("Root span's ParentID %d is not 0", exp.ParentID())
	}

	for i, sp := range spans {
		if a, e := sp.ParentID(), exp.SpanID(); a != e {
			t.Fatalf("%d: ParentID %d does not match expectation %d", i, a, e)
		} else {
			// Prepare for comparison.
			sp.TraceContext.spanID, sp.TraceContext.parentID = exp.SpanID(), 0
			sp.duration, sp.startTime = exp.duration, exp.startTime
		}
		if a, e := sp.TraceID(), exp.TraceID(); a != e {
			t.Fatalf("%d: TraceID changed from %d to %d", i, e, a)
		}
		if !reflect.DeepEqual(exp.TraceContext, sp.TraceContext) {
			t.Fatalf("%d: wanted %+v, got %+v", i, exp.TraceContext, sp.TraceContext)
		}
		if !reflect.DeepEqual(exp.baggage, sp.baggage) {
			t.Fatalf("%d: wanted %+v, got %+v", i, exp.baggage, sp.baggage)
		}
		// Strictly speaking, the above two checks are not necessary, but they are helpful
		// for troubleshooting when something does not compare equal.
		if !reflect.DeepEqual(exp, sp) {
			t.Fatalf("%d: wanted %+v, got %+v", i, exp, sp)
		}
	}

	assert.EqualValues(t, map[string]int64{
		"jaeger.spans|group=sampling|sampled=y":       7,
		"jaeger.spans|group=lifecycle|state=started":  7,
		"jaeger.spans|group=lifecycle|state=finished": 4,
		"jaeger.traces|sampled=y|state=started":       1,
		"jaeger.traces|sampled=y|state=joined":        3,
	}, stats.GetCounterValues())
}

func TestSpanIntegrityAfterSerialize(t *testing.T) {
	serializedString := "f6c385a2c57ed8d7:b04a90b7723bdc:76c385a2c57ed8d7:1"

	context, err := ContextFromString(serializedString)
	require.NoError(t, err)
	require.True(t, context.traceID > (uint64(1)<<63))
	require.True(t, int64(context.traceID) < 0)

	newSerializedString := context.String()
	require.Equal(t, serializedString, newSerializedString)
}

func TestDecodingError(t *testing.T) {
	reporter := NewInMemoryReporter()
	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)
	tracer, closer := NewTracer("x", NewConstSampler(true), reporter, TracerOptions.Metrics(metrics))
	defer closer.Close()

	badHeader := "x.x.x.x"
	httpHeader := http.Header{}
	httpHeader.Add(TracerStateHeaderName, badHeader)
	tmc := opentracing.HTTPHeaderTextMapCarrier(httpHeader)
	_, err := tracer.Join("test", opentracing.TextMap, tmc)
	assert.Error(t, err)
	assert.Equal(t, map[string]int64{"jaeger.decoding-errors": 1}, stats.GetCounterValues())
}

func TestBaggagePropagationHTTP(t *testing.T) {
	tracer, closer := NewTracer("DOOP", NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*span)
	sp1.SetBaggageItem("Some_Key", "12345")
	assert.Equal(t, "12345", sp1.BaggageItem("some-KEY"))
	sp1.SetBaggageItem("Some_Key", "98765")
	assert.Equal(t, "98765", sp1.BaggageItem("some-KEY"))

	h := http.Header{}
	err := tracer.Inject(sp1, opentracing.TextMap, opentracing.HTTPHeaderTextMapCarrier(h))
	require.NoError(t, err)

	sp2, err := tracer.Join("x", opentracing.TextMap, opentracing.HTTPHeaderTextMapCarrier(h))
	require.NoError(t, err)
	assert.Equal(t, "98765", sp2.BaggageItem("some-KEY"))
}
