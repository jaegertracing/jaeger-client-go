package jaeger

import (
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
)

func TestFirstInProcessSpan(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNoopReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*span)
	sp2 := opentracing.StartChildSpan(sp1, "sp2").(*span)
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
