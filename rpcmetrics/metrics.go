package rpcmetrics

import (
	"sync"

	"github.com/uber/jaeger-lib/metrics"
)

const (
	otherEndpointsPlaceholder = "other"
	endpointNameMetricTag     = "endpoint"
)

// Metrics is a collection of metrics for an endpoint describing
// throughput, success, errors, and performance
type Metrics struct {
	// Requests is a counter of the total number of successes + failures.
	Requests metrics.Counter `metric:"requests"`

	// Success is a counter of the total number of successes.
	Success metrics.Counter `metric:"success"`

	// Failures is a counter of the number of times any failure has been observed.
	Failures metrics.Counter `metric:"failures"`

	// RequestLatencyMs is a histogram of the latency of requests in milliseconds.
	RequestLatencyMs metrics.Timer `metric:"request_latency_ms"`

	// HTTPStatusCode2xx is a counter of the total number of requests with HTTP status code 200-299
	HTTPStatusCode2xx metrics.Counter `metric:"http_status_code_2xx"`

	// HTTPStatusCode3xx is a counter of the total number of requests with HTTP status code 300-399
	HTTPStatusCode3xx metrics.Counter `metric:"http_status_code_3xx"`

	// HTTPStatusCode4xx is a counter of the total number of requests with HTTP status code 400-499
	HTTPStatusCode4xx metrics.Counter `metric:"http_status_code_4xx"`

	// HTTPStatusCode5xx is a counter of the total number of requests with HTTP status code 500-599
	HTTPStatusCode5xx metrics.Counter `metric:"http_status_code_5xx"`
}

func (m *Metrics) recordHTTPStatusCode(statusCode uint16) {
	if statusCode >= 200 && statusCode < 300 {
		m.HTTPStatusCode2xx.Inc(1)
	} else if statusCode >= 300 && statusCode < 400 {
		m.HTTPStatusCode3xx.Inc(1)
	} else if statusCode >= 400 && statusCode < 500 {
		m.HTTPStatusCode4xx.Inc(1)
	} else if statusCode >= 500 && statusCode < 600 {
		m.HTTPStatusCode5xx.Inc(1)
	}
}

// MetricsByEndpoint is a registry of metrics for each unique endpoint name
type MetricsByEndpoint struct {
	metricsFactory    metrics.Factory
	endpoints         *normalizedEndpoints
	metricsByEndpoint map[string]*Metrics
	mux               sync.RWMutex
}

func newMetricsByEndpoint(
	metricsFactory metrics.Factory,
	normalizer NameNormalizer,
	maxNumberOfEndpoints int,
) *MetricsByEndpoint {
	return &MetricsByEndpoint{
		metricsFactory:    metricsFactory,
		endpoints:         newNormalizedEndpoints(maxNumberOfEndpoints, normalizer),
		metricsByEndpoint: make(map[string]*Metrics, maxNumberOfEndpoints+1), // +1 for "other"
	}
}

func (m *MetricsByEndpoint) get(endpoint string) *Metrics {
	safeName := m.endpoints.normalize(endpoint)
	if safeName == "" {
		safeName = otherEndpointsPlaceholder
	}
	m.mux.RLock()
	met := m.metricsByEndpoint[safeName]
	m.mux.RUnlock()
	if met != nil {
		return met
	}

	return m.getWithWriteLock(safeName)
}

// split to make easier to test
func (m *MetricsByEndpoint) getWithWriteLock(safeName string) *Metrics {
	m.mux.Lock()
	defer m.mux.Unlock()

	// it is possible that the name has been already registered after we released
	// the read lock and before we grabbed the write lock, so check for that.
	if met, ok := m.metricsByEndpoint[safeName]; ok {
		return met
	}

	// it would be nice to create the struct before locking, since Init() is somewhat
	// expensive, however some metrics backend (e.g. expvar) may not like duplicate metrics.
	met := &Metrics{}
	tags := map[string]string{endpointNameMetricTag: safeName}
	metrics.Init(met, m.metricsFactory, tags)

	m.metricsByEndpoint[safeName] = met
	return met
}
