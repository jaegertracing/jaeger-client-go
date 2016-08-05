package jaeger

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanPropagator(t *testing.T) {
	const op = "test"
	reporter := NewInMemoryReporter()
	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)
	tracer, closer := NewTracer("x", NewConstSampler(true), reporter, TracerOptions.Metrics(metrics))

	mapc := opentracing.TextMapCarrier(make(map[string]string))
	httpc := opentracing.HTTPHeadersCarrier(http.Header{})
	tests := []struct {
		format, carrier, formatName interface{}
	}{
		{SpanContextFormat, new(SpanContext), "TraceContextFormat"},
		{opentracing.Binary, new(bytes.Buffer), "Binary"},
		{opentracing.TextMap, mapc, "TextMap"},
		{opentracing.HTTPHeaders, httpc, "HTTPHeaders"},
	}

	sp := tracer.StartSpan(op)
	sp.SetTag("x", "y") // to avoid later comparing nil vs. []
	sp.SetBaggageItem("foo", "bar")
	for _, test := range tests {
		// starting normal child to extract its serialized context
		child := tracer.StartSpan(op, opentracing.ChildOf(sp.Context()))
		err := tracer.Inject(child.Context(), test.format, test.carrier)
		assert.NoError(t, err)
		// Note: we're not finishing the above span
		childCtx, err := tracer.Extract(test.format, test.carrier)
		assert.NoError(t, err)
		child = tracer.StartSpan(test.formatName.(string), ext.RPCServerOption(childCtx))
		child.SetTag("x", "y") // to avoid later comparing nil vs. []
		child.Finish()
	}
	sp.Finish()
	closer.Close()

	otSpans := reporter.GetSpans()
	require.Equal(t, len(tests)+1, len(otSpans), "unexpected number of spans reporter")

	spans := make([]*span, len(otSpans))
	for i, s := range otSpans {
		spans[i] = s.(*span)
	}

	// The last span is the original one.
	exp, spans := spans[len(spans)-1], spans[:len(spans)-1]
	exp.duration = time.Duration(123)
	exp.startTime = time.Time{}.Add(1)

	if exp.context.ParentID() != 0 {
		t.Fatalf("Root span's ParentID %d is not 0", exp.context.ParentID())
	}

	for i, sp := range spans {
		formatName := sp.operationName
		if a, e := sp.context.ParentID(), exp.context.SpanID(); a != e {
			t.Fatalf("%d: ParentID %d does not match expectation %d", i, a, e)
		} else {
			// Prepare for comparison.
			sp.context.spanID, sp.context.parentID = exp.context.SpanID(), 0
			sp.duration, sp.startTime = exp.duration, exp.startTime
		}
		assert.Equal(t, exp.context, sp.context, formatName)
		assert.Equal(t, exp.tags, sp.tags, formatName)
		assert.Equal(t, exp.logs, sp.logs, formatName)
		assert.EqualValues(t, "server", sp.spanKind, formatName)
		// Override collections to avoid tripping comparison on different pointers
		sp.context = exp.context
		sp.tags = exp.tags
		sp.logs = exp.logs
		sp.spanKind = exp.spanKind
		sp.operationName = op
		// Compare the rest of the fields
		assert.Equal(t, exp, sp, formatName)
	}

	assert.EqualValues(t, map[string]int64{
		"jaeger.spans|group=sampling|sampled=y":       int64(1 + 2*len(tests)),
		"jaeger.spans|group=lifecycle|state=started":  int64(1 + 2*len(tests)),
		"jaeger.spans|group=lifecycle|state=finished": int64(1 + len(tests)),
		"jaeger.traces|sampled=y|state=started":       1,
		"jaeger.traces|sampled=y|state=joined":        int64(len(tests)),
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
	tmc := opentracing.HTTPHeadersCarrier(httpHeader)
	_, err := tracer.Extract(opentracing.HTTPHeaders, tmc)
	assert.Error(t, err)
	assert.Equal(t, map[string]int64{"jaeger.decoding-errors": 1}, stats.GetCounterValues())
}

func TestBaggagePropagationHTTP(t *testing.T) {
	tracer, closer := NewTracer("DOOP", NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1")
	sp1.SetBaggageItem("Some_Key", "12345")
	assert.Equal(t, "12345", sp1.BaggageItem("some-KEY"), "baggage: %+v", sp1.(*span).context.baggage)
	sp1.SetBaggageItem("Some_Key", "98765")
	assert.Equal(t, "98765", sp1.BaggageItem("some-KEY"), "baggage: %+v", sp1.(*span).context.baggage)

	h := http.Header{}
	err := tracer.Inject(sp1.Context(), opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(h))
	require.NoError(t, err)

	sp2, err := tracer.Extract(opentracing.HTTPHeaders, opentracing.HTTPHeadersCarrier(h))
	require.NoError(t, err)
	assert.Empty(t, sp2.(SpanContext).baggage["some-KEY"])
	assert.Equal(t, "98765", sp2.(SpanContext).baggage["some-key"])
}
