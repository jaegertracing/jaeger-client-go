// Copyright (c) 2017 Uber Technologies, Inc.
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
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Metrics is a container of all stats emitted by Jaeger tracer.
type Metrics struct {
	// Number of traces started by this tracer as sampled
	TracesStartedSampled *counter `metric:"traces" tags:"state=started,sampled=y"`

	// Number of traces started by this tracer as not sampled
	TracesStartedNotSampled *counter `metric:"traces" tags:"state=started,sampled=n"`

	// Number of externally started sampled traces this tracer joined
	TracesJoinedSampled *counter `metric:"traces" tags:"state=joined,sampled=y"`

	// Number of externally started not-sampled traces this tracer joined
	TracesJoinedNotSampled *counter `metric:"traces" tags:"state=joined,sampled=n"`

	// Number of sampled spans started by this tracer
	SpansStarted *counter `metric:"spans" tags:"group=lifecycle,state=started"`

	// Number of sampled spans finished by this tracer
	SpansFinished *counter `metric:"spans" tags:"group=lifecycle,state=finished"`

	// Number of sampled spans started by this tracer
	SpansSampled *counter `metric:"spans" tags:"group=sampling,sampled=y"`

	// Number of not-sampled spans started by this tracer
	SpansNotSampled *counter `metric:"spans" tags:"group=sampling,sampled=n"`

	// Number of errors decoding tracing context
	DecodingErrors *counter `metric:"decoding-errors"`

	// Number of spans successfully reported
	ReporterSuccess *counter `metric:"reporter-spans" tags:"state=success"`

	// Number of spans in failed attempts to report
	ReporterFailure *counter `metric:"reporter-spans" tags:"state=failure"`

	// Number of spans dropped due to internal queue overflow
	ReporterDropped *counter `metric:"reporter-spans" tags:"state=dropped"`

	// Current number of spans in the reporter queue
	ReporterQueueLength *gauge `metric:"reporter-queue"`

	// Number of times the Sampler succeeded to retrieve sampling strategy
	SamplerRetrieved *counter `metric:"sampler" tags:"state=retrieved"`

	// Number of times the Sampler succeeded to retrieve and update sampling strategy
	SamplerUpdated *counter `metric:"sampler" tags:"state=updated"`

	// Number of times the Sampler failed to update sampling strategy
	SamplerUpdateFailure *counter `metric:"sampler" tags:"state=failure,phase=updating"`

	// Number of times the Sampler failed to retrieve sampling strategy
	SamplerQueryFailure *counter `metric:"sampler" tags:"state=failure,phase=query"`

	// Number of times the Sampler failed to parse retrieved sampling strategy
	SamplerParsingFailure *counter `metric:"sampler" tags:"state=failure,phase=parsing"`
}

// NewMetrics creates a new Metrics struct and initializes its fields using reflection.
func NewMetrics(reporter StatsReporter, globalTags map[string]string) *Metrics {
	m := &Metrics{}
	if err := initMetrics(m, reporter, globalTags); err != nil {
		panic(err.Error())
	}
	return m
}

// stat is a container of stats identity.
// It is used by counter, gauge, and timer structs that each expose a single update method.
type stat struct {
	name     string
	tags     map[string]string
	reporter StatsReporter
}

type counter struct {
	stat
}

func (c *counter) Inc(delta int64) {
	c.reporter.IncCounter(c.name, c.tags, delta)
}

type gauge struct {
	stat
}

func (c *gauge) Update(value int64) {
	c.reporter.UpdateGauge(c.name, c.tags, value)
}

type timer struct {
	stat
}

func (c *timer) Record(d time.Duration) {
	c.reporter.RecordTimer(c.name, c.tags, d)
}

// initMetrics uses reflection to initialize a struct containing metrics fields
// by assigning new Counter/Gauge/Timer values with the metric name retrieved
// from the `metric` tag and stats tags retrieved from the `tags` tag.
//
// Note: all fields of the struct must be exported, have a `metric` tag, and be
// of type *counter or *gauge or *timer.
func initMetrics(m *Metrics, reporter StatsReporter, globalTags map[string]string) error {
	// Allow user to opt out of reporting metrics by passing in nil.
	if reporter == nil {
		reporter = NullStatsReporter
	}

	// We prefix all stat names with "jaeger.", because in go-common we do not know
	// what kind of backend we're going to get from config, and if it happens to be
	// statsd backend, it will drop all tags and we will end up with all stats called
	// "span" or trace", or "decoding-errors", which has the risk of conflicting with
	// the end users's own metrics.
	const prefix = "jaeger."
	counterPtrType := reflect.TypeOf(&counter{})
	gaugePtrType := reflect.TypeOf(&gauge{})
	timerPtrType := reflect.TypeOf(&timer{})

	v := reflect.ValueOf(m).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		tags := make(map[string]string)
		for k, v := range globalTags {
			tags[k] = v
		}
		field := t.Field(i)
		metric := field.Tag.Get("metric")
		if metric == "" {
			return fmt.Errorf("Field %s is missing a tag 'metric'", field.Name)
		}
		if tagString := field.Tag.Get("tags"); tagString != "" {
			tagPairs := strings.Split(tagString, ",")
			for _, tagPair := range tagPairs {
				tag := strings.Split(tagPair, "=")
				if len(tag) != 2 {
					return fmt.Errorf(
						"Field [%s]: Tag [%s] is not of the form key=value in 'tags' string [%s] ",
						field.Name, tagPair, tagString)
				}
				tags[tag[0]] = tag[1]
			}
		}
		stat := stat{name: prefix + metric, tags: tags, reporter: reporter}
		var obj interface{}
		if field.Type.AssignableTo(counterPtrType) {
			obj = &counter{stat}
		} else if field.Type.AssignableTo(gaugePtrType) {
			obj = &gauge{stat}
		} else if field.Type.AssignableTo(timerPtrType) {
			obj = &timer{stat}
		} else {
			return fmt.Errorf(
				"Field %s is is not a pointer to timer, gauge, or counter",
				field.Name)
		}
		v.Field(i).Set(reflect.ValueOf(obj))
	}
	return nil
}
