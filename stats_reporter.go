// Copyright (c) 2016 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

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
	sync.Mutex
	counters map[string]*inMemoryCounter
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

// MetricDescr describes a metric with tags
type MetricDescr struct {
	Name string
	Tags map[string]string
}

// NewMetricDescr creates a new metric descriptor from the name and an even number
// of keyValues strings treates as alternating key, value pairs.
func NewMetricDescr(name string, keyValues ...string) MetricDescr {
	m := MetricDescr{Name: name}
	if len(keyValues)%2 != 0 {
		return m.WithTag("error", "uneven_keyValues_count")
	}
	for i := 0; i < len(keyValues); i += 2 {
		m = m.WithTag(keyValues[i], keyValues[i+1])
	}
	return m
}

// WithTag returns a new metric descriptor with additional tag
func (m MetricDescr) WithTag(key, value string) MetricDescr {
	tags := m.Tags
	if tags == nil {
		tags = make(map[string]string)
	}
	tags[key] = value
	return MetricDescr{
		Name: m.Name,
		Tags: tags,
	}
}

// Key converts name+tags into a single string of the form
// "name|tag1=value1|...|tagN=valueN", where tag names are
// sorted alphabetically.
func (m MetricDescr) Key() string {
	var keys []string
	for k := range m.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	key := m.Name
	for _, k := range keys {
		key = key + "|" + k + "=" + m.Tags[k]
	}
	return key
}

func (r *InMemoryStatsCollector) getKey(name string, tags map[string]string) string {
	return MetricDescr{
		Name: name,
		Tags: tags,
	}.Key()
}

// IncCounter implements IncCounter of StatsReporter
func (r *InMemoryStatsCollector) IncCounter(name string, tags map[string]string, value int64) {
	key := r.getKey(name, tags)
	r.Lock()
	defer r.Unlock()
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
	r.Lock()
	defer r.Unlock()
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

// Clear discards accumulated stats
func (r *InMemoryStatsCollector) Clear() {
	r.Lock()
	defer r.Unlock()
	r.counters = make(map[string]*inMemoryCounter)
}

// GetCounterValue retrieves the value of a counter with the given name and tags.
// Tags are read from an even number of key-values strings treates as alternating
// key, value pairs.
func (r *InMemoryStatsCollector) GetCounterValue(name string, tags ...string) int64 {
	r.Lock()
	defer r.Unlock()
	descr := NewMetricDescr(name, tags...)
	counter := r.counters[descr.Key()]
	if counter == nil {
		return 0
	}
	return counter.Value
}
