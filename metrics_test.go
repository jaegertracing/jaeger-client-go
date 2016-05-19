package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMetrics(t *testing.T) {
	tags := map[string]string{"lib": "jaeger"}

	m := &Metrics{}
	err := initMetrics(m, nil, tags)
	require.NoError(t, err)

	m = NewMetrics(nil, tags)
	require.NotNil(t, m.SpansSampled, "counter not initialized")
	require.NotNil(t, m.ReporterQueueLength, "gauge not initialized")
	require.NotEmpty(t, m.SpansSampled.tags)
	assert.Equal(t, "jaeger.spans", m.SpansSampled.name)
	assert.Equal(t, "sampling", m.SpansSampled.tags["group"])
	assert.Equal(t, "y", m.SpansSampled.tags["sampled"])
	assert.Equal(t, "jaeger", m.SpansSampled.tags["lib"])
}
