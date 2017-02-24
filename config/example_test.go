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

package config_test

import (
	"log"

	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger-client-go"
	. "github.com/uber/jaeger-client-go/config"
	jaeger_log "github.com/uber/jaeger-client-go/log"
)

func ExampleInitialization() {
	cfg := Configuration{
		Disabled: false,
		Sampler: &SamplerConfig{
			Type:  jaeger.SamplerTypeProbabilistic,
			Param: 0.001,
		},
	}

	// Initialize tracer with a logger and a metrics factory
	closer, err := cfg.InitGlobalTracer("serviceName", Logger(newLogger()), Metrics(newMetricsFactory()))
	if err != nil {
		log.Printf("Could not initialize jaeger tracer: %s", err.Error())
		return
	}
	defer closer.Close()
}

// yourLogger is an implementation of the jaeger-client-go/log#Logger interface that
// delegates to default `log` package.
// jaeger-client-go has a Logger wrapper for zap ("github.com/uber/jaeger-client-go/log/zap")
type yourLogger struct{}

func newLogger() jaeger_log.Logger {
	return &yourLogger{}
}

// Error logs a message at error priority
func (l *yourLogger) Error(msg string) {
	log.Printf("ERROR: %s", msg)
}

// Infof logs a message at info priority
func (l *yourLogger) Infof(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

// yourMetricsFactory is an implementation of jaeger-lib/metrics#Factory that returns
// NullCounter, NullTimer, and NullGauge.
// jaeger-lib has metrics factories for go-kit ("github.com/uber/jaeger-lib/metrics/go-kit") and
// tally ("github.com/uber/jaeger-lib/metrics/tally")
type yourMetricsFactory struct{}

func newMetricsFactory() metrics.Factory {
	return &yourMetricsFactory{}
}

func (yourMetricsFactory) Counter(name string, tags map[string]string) metrics.Counter {
	return metrics.NullCounter
}

func (yourMetricsFactory) Timer(name string, tags map[string]string) metrics.Timer {
	return metrics.NullTimer
}

func (yourMetricsFactory) Gauge(name string, tags map[string]string) metrics.Gauge {
	return metrics.NullGauge
}

func (yourMetricsFactory) Namespace(name string, tags map[string]string) metrics.Factory {
	return metrics.NullFactory
}
