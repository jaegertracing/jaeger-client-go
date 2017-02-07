package rpcmetrics

import (
	"sync"

	"github.com/uber/jaeger-lib/metrics"
)

const defaultMaxNumberOfEndpoints = 200

// Metrics is a collection of metrics for an endpoint describing
// throughput, success, errors, and performance
type Metrics struct {
	// Requests is a counter of the total number of successes + failures.
	Requests metrics.Counter `metric:"requests"`

	// Success is a counter of the total number of successes.
	Success metrics.Counter `metric:"success"`

	// Failures is a counter of the number of times any failure has been observed.
	Failures metrics.Counter `metric:"failures"`

	// Pending is a guage of the current total number of outstanding requests.
	Pending metrics.Gauge `metric:"pending"`

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

// MetricsByEndpoint is a registry of metrics for each unique endpoint name
type MetricsByEndpoint struct {
	metricsFactory       metrics.Factory
	endpoints            *normalizedEndpoints
	metricsByEndpoint    map[string]*Metrics
	maxNumberOfEndpoints int
	mux                  sync.RWMutex
}

func newMetricsByEndpoint(metricsFactory metrics.Factory) *MetricsByEndpoint {
	return &MetricsByEndpoint{
		metricsFactory:       metricsFactory,
		endpoints:            newNormalizedEndpoints(defaultMaxNumberOfEndpoints, ""),
		metricsByEndpoint:    make(map[string]*Metrics),
		maxNumberOfEndpoints: 200,
	}
}

func (m *MetricsByEndpoint) get(endpoint string) *Metrics {
	safeName := m.endpoints.normalize(endpoint)
	if safeName == "" {
		safeName = "other"
	}
	m.mux.RLock()
	met := m.metricsByEndpoint[safeName]
	m.mux.RUnlock()
	if met != nil {
		return met
	}

	m.mux.Lock()
	defer m.mux.Unlock()

	if met, ok := m.metricsByEndpoint[safeName]; ok {
		return met // created since we released read lock
	}

	if len(m.metricsByEndpoint) >= m.maxNumberOfEndpoints {
		safeName = "other"
	}

	if met, ok := m.metricsByEndpoint[safeName]; ok {
		return met // couldn've been created earlier
	}

	met = &Metrics{}
	tags := map[string]string{"endpoint": safeName}
	metrics.Init(met, m.metricsFactory, tags)
	m.metricsByEndpoint[safeName] = met
	return met
}
