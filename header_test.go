package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaultOrCustom(t *testing.T) {
	assert.Equal(t, (&HeadersConfig{}).SetDefaultOrCustom(), getDefaultHeadersConfig())

	assert.Equal(t, (&HeadersConfig{
		JaegerDebugHeader: "custom-jaeger-debug-header",
	}).SetDefaultOrCustom(), &HeadersConfig{
		JaegerDebugHeader:        "custom-jaeger-debug-header",
		JaegerBaggageHeader:      JaegerBaggageHeader,
		TracerStateHeaderName:    TracerStateHeaderName,
		TraceBaggageHeaderPrefix: TraceBaggageHeaderPrefix,
	})

	customHeaders := &HeadersConfig{
		JaegerDebugHeader:        "custom-jaeger-debug-header",
		JaegerBaggageHeader:      "custom-jaeger-baggage-header",
		TracerStateHeaderName:    "custom-tracer-state-header-name",
		TraceBaggageHeaderPrefix: "custom-tracer-baggage-header-prefix",
	}
	assert.Equal(t, customHeaders.SetDefaultOrCustom(), customHeaders)
}

func TestGetDefaultHeadersConfig(t *testing.T) {
	assert.Equal(t, getDefaultHeadersConfig(), &HeadersConfig{
		JaegerDebugHeader:        JaegerDebugHeader,
		JaegerBaggageHeader:      JaegerBaggageHeader,
		TracerStateHeaderName:    TracerStateHeaderName,
		TraceBaggageHeaderPrefix: TraceBaggageHeaderPrefix,
	})
}
