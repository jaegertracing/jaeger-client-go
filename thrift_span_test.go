package jaeger

import (
	"fmt"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger-client-go/thrift-gen/zipkincore"
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
		span     *span
		wantTags bool
	}{
		{sp1, true},
		{sp2, false},
	}

	for _, test := range tests {
		var check func(assert.TestingT, interface{}, ...interface{}) bool
		if test.wantTags {
			check = assert.NotNil
		} else {
			check = assert.Nil
		}
		thriftSpan := buildThriftSpan(test.span)
		version := findBinaryAnnotation(thriftSpan, JaegerClientTag)
		hostname := findBinaryAnnotation(thriftSpan, TracerHostnameKey)
		check(t, version)
		check(t, hostname)
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

	tests := []struct {
		addComponentTag bool
		wantAnnotation  string
	}{
		{false, "DOOP"}, // Without COMPONENT tag the value is the service name
		{true, "c1"},
	}

	for _, test := range tests {
		sp := tracer.StartSpan("s1").(*span)
		if test.addComponentTag {
			ext.Component.Set(sp, "c1")
		}
		sp.Finish()
		thriftSpan := buildThriftSpan(sp)

		anno := findBinaryAnnotation(thriftSpan, "lc")
		assert.NotNil(t, anno)
		assert.EqualValues(t, test.wantAnnotation, anno.Value)
	}
}

func findAnnotation(span *zipkincore.Span, name string) *zipkincore.Annotation {
	for _, a := range span.Annotations {
		if a.Value == name {
			return a
		}
	}
	return nil
}

func findBinaryAnnotation(span *zipkincore.Span, name string) *zipkincore.BinaryAnnotation {
	for _, a := range span.BinaryAnnotations {
		if a.Key == name {
			return a
		}
	}
	return nil
}
