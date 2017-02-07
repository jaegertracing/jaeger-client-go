package rpcmetrics

import (
	"sync"
	"sync/atomic"

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

	pendingCount int64
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

func (m *Metrics) updatePendingCount(delta int64) {
	pending := atomic.AddInt64(&m.pendingCount, delta)
	m.Pending.Update(pending)
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
