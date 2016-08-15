package jaeger

import (
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
