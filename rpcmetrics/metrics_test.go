package rpcmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"
)

func endpointTags(endpoint string) map[string]string {
	return map[string]string{
		"endpoint": endpoint,
	}
}

func TestMetricsByEndpoint(t *testing.T) {
	met := metrics.NewLocalFactory(0)
	mbe := newMetricsByEndpoint(met, DefaultNameNormalizer, 2)

	m1 := mbe.get("abc1")
	m2 := mbe.get("abc1")               // from cache
	m2a := mbe.getWithWriteLock("abc1") // from cache in double-checked lock
	assert.Equal(t, m1, m2)
	assert.Equal(t, m1, m2a)

	m3 := mbe.get("abc3")
	m4 := mbe.get("overflow")
	m5 := mbe.get("overflow2")

	for _, m := range []*Metrics{m1, m2, m2a, m3, m4, m5} {
		m.Requests.Inc(1)
	}

	testutils.AssertCounterMetrics(t, met,
		testutils.ExpectedMetric{Name: "requests", Tags: endpointTags("abc1"), Value: 3},
		testutils.ExpectedMetric{Name: "requests", Tags: endpointTags("abc3"), Value: 1},
		testutils.ExpectedMetric{Name: "requests", Tags: endpointTags("other"), Value: 2},
	)
}
