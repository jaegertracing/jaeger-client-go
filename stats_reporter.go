package jaeger

import (
	"sort"
	"sync"
	"time"
)

// StatsReporter is an interface for statsd-like stats reporters accepted by Uber's libraries.
// Its methods take optional tag dictionaries which may be ignored by concrete implementations.
type StatsReporter interface {
	// Increment a statsd-like counter with optional tags
	IncCounter(name string, tags map[string]string, value int64)

	// Increment a statsd-like gauge ("set" of the value) with optional tags
	UpdateGauge(name string, tags map[string]string, value int64)

	// Record a statsd-like timer with optional tags
	RecordTimer(name string, tags map[string]string, d time.Duration)
}

// NullStatsReporter is a stats reporter that discards the statistics.
var NullStatsReporter StatsReporter = nullStatsReporter{}

type nullStatsReporter struct{}

func (nullStatsReporter) IncCounter(name string, tags map[string]string, value int64)      {}
func (nullStatsReporter) UpdateGauge(name string, tags map[string]string, value int64)     {}
func (nullStatsReporter) RecordTimer(name string, tags map[string]string, d time.Duration) {}

// InMemoryStatsCollector collects all stats in-memory and provides access to them via snapshots.
// Currently only the counters are implemented.
//
// This is only meant for testing, not optimized for production use.
type InMemoryStatsCollector struct {
	counters map[string]*inMemoryCounter
	cMutex   sync.Mutex
}

// NewInMemoryStatsCollector creates new in-memory stats reporter (aggregator)
func NewInMemoryStatsCollector() *InMemoryStatsCollector {
	return &InMemoryStatsCollector{counters: make(map[string]*inMemoryCounter)}
}

// InMemoryCounter represents a value of a single counter, along with its stat name and tags
type inMemoryCounter struct {
	Name  string
	Value int64
	Tags  map[string]string
}

// getKey converts name+tags into a single string of the form
// "name|tag1=value1|...|tagN=valueN", where tag names are sorted alphabetically.
func (r *InMemoryStatsCollector) getKey(name string, tags map[string]string) string {
	var keys []string
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	key := name
	for _, k := range keys {
		key = key + "|" + k + "=" + tags[k]
	}
	return key
}

// IncCounter implements IncCounter of StatsReporter
func (r *InMemoryStatsCollector) IncCounter(name string, tags map[string]string, value int64) {
	key := r.getKey(name, tags)
	r.cMutex.Lock()
	defer r.cMutex.Unlock()
	if entry, ok := r.counters[key]; ok {
		entry.Value += value
	} else {
		r.counters[key] = &inMemoryCounter{
			Name:  name,
			Value: value,
			Tags:  tags}
	}
}

// GetCounterValues returns a snapshot of currently accumulated counter values.
// The keys in the map are in the form "name|tag1=value1|...|tagN=valueN", where
// tag names are sorted alphabetically.
func (r *InMemoryStatsCollector) GetCounterValues() map[string]int64 {
	r.cMutex.Lock()
	defer r.cMutex.Unlock()
	res := make(map[string]int64, len(r.counters))
	for k, v := range r.counters {
		res[k] = v.Value
	}
	return res
}

// UpdateGauge is a no-op implementation of UpdateGauge of StatsReporter
func (r *InMemoryStatsCollector) UpdateGauge(name string, tags map[string]string, value int64) {}

// RecordTimer is a no-op implementation of RecordTimer of StatsReporter
func (r *InMemoryStatsCollector) RecordTimer(name string, tags map[string]string, d time.Duration) {}
