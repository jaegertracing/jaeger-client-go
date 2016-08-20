package jaeger

import (
	"fmt"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
)

func TestThriftFirstInProcessSpan(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*span)
	sp2 := tracer.StartSpan("sp2", opentracing.ChildOf(sp1.Context())).(*span)
	sp2.Finish()
	sp1.Finish()

	tests := []struct {
		span       *span
		versionTag bool
	}{
		{sp1, true},
		{sp2, false},
	}

	for _, test := range tests {
		thriftSpan := buildThriftSpan(test.span)
		jaegerClientTagFound := false
		for _, anno := range thriftSpan.BinaryAnnotations {
			jaegerClientTagFound = jaegerClientTagFound || (anno.Key == JaegerClientTag)
		}
		assert.Equal(t, test.versionTag, jaegerClientTagFound)
	}
}

func TestThriftForceSampled(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(false), // sample nothing
		NewNullReporter())
	defer closer.Close()

	sp := tracer.StartSpan("s1").(*span)
	ext.SamplingPriority.Set(sp, 1)
	assert.True(t, sp.context.IsSampled())
	assert.True(t, sp.context.IsDebug())
	thriftSpan := buildThriftSpan(sp)
	assert.True(t, thriftSpan.Debug)
}

func TestThriftSpanLogs(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()

	sp := tracer.StartSpan("s1").(*span)
	payload := "luggage"
	n := len(logPayloadLabels)
	m := n + 5
	for i := 0; i < m; i++ {
		sp.LogEventWithPayload(fmt.Sprintf("event%d", i), payload)
	}
	thriftSpan := buildThriftSpan(sp)
	var (
		logs             int
		numberedPayloads int
		plainPayloads    int
	)
	for i := 0; i < m; i++ {
		for _, anno := range thriftSpan.Annotations {
			if anno.Value == fmt.Sprintf("event%d", i) {
				logs++
			}
		}
		for _, anno := range thriftSpan.BinaryAnnotations {
			if anno.Key == fmt.Sprintf("log_payload_%d", i) {
				numberedPayloads++
			}
		}
	}
	for _, anno := range thriftSpan.BinaryAnnotations {
		if anno.Key == "log_payload" {
			plainPayloads++
		}
	}
	assert.Equal(t, m, logs, "Each log must create Annotation")
	assert.Equal(t, n, numberedPayloads, "Each log must create numbered BinaryAnnotation")
	assert.Equal(t, m-n, plainPayloads, "Each log over %d must create unnumbered BinaryAnnotation", n)
}

func TestThriftLocalComponentSpan(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()

	sp := tracer.StartSpan("s1").(*span)
	sp.Finish()
	thriftSpan := buildThriftSpan(sp)

	anno := thriftSpan.BinaryAnnotations[len(thriftSpan.BinaryAnnotations)-1]
	assert.Equal(t, "lc", anno.Key)
	assert.EqualValues(t, "DOOP", anno.Value, "Without COMPONENT tag the value is service name")

	sp = tracer.StartSpan("s1").(*span)
	ext.Component.Set(sp, "c1")
	sp.Finish()
	thriftSpan = buildThriftSpan(sp)

	anno = thriftSpan.BinaryAnnotations[len(thriftSpan.BinaryAnnotations)-2]
	assert.Equal(t, "lc", anno.Key)
	assert.EqualValues(t, "c1", anno.Value, "Value of COMPONENT tag")
}
