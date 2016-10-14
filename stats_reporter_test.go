package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type expectedMetric struct {
	metric []string
	value  int
}

func assertMetrics(t *testing.T, stats *InMemoryStatsCollector, expectedMetrics []expectedMetric) {
	for _, expected := range expectedMetrics {
		assert.EqualValues(t, expected.value,
			stats.GetCounterValue(expected.metric[0], expected.metric[1:]...),
			"metric %+v", expected.metric,
		)
	}
	assert.Len(t, stats.GetCounterValues(), len(expectedMetrics))
}
