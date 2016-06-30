package jaeger

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestBaggageIterator(t *testing.T) {
	tracer, closer := NewTracer("DOOP", NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("s1").(*span)
	sp1.SetBaggageItem("Some_Key", "12345")
	sp1.SetBaggageItem("Some-other-key", "42")

	b := make(map[string]string)
	sp1.ForeachBaggageItem(func(k, v string) bool {
		b[k] = v
		return true
	})
	assert.Equal(t, map[string]string{"some-key": "12345", "some-other-key": "42"}, b)

	b = make(map[string]string)
	sp1.ForeachBaggageItem(func(k, v string) bool {
		b[k] = v
		return false // break out early
	})
	assert.Equal(t, 1, len(b), "only one baggage item should be extracted")
}
