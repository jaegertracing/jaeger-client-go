// Copyright (c) 2016 Uber Technologies, Inc.

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

package config

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/opentracing/opentracing-go"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/transport"
	"github.com/uber/jaeger-client-go/transport/udp"
)

// Configuration configures and creates Jaeger Tracer
type Configuration struct {
	Disabled bool            `yaml:"disabled"`
	Sampler  *SamplerConfig  `yaml:"sampler"`
	Reporter *ReporterConfig `yaml:"reporter"`

	// Logger can be provided to log Reporter errors, as well as to log spans
	// if Reporter.LogSpans is set to true. This cannot be specified in
	// the configuration file, it must be specified in code.
	Logger jaeger.Logger `yaml:"-"`
}

// SamplerConfig allows initializing a non-default sampler.  All fields are optional.
type SamplerConfig struct {
	// Type specifies the type of the sampler: const, probabilistic, rateLimiting, or remote
	Type string `yaml:"type"`

	// Param is a value passed to the sampler.
	// Valid values for Param field are:
	// - for "const" sampler, 0 or 1 for always false/true respectively
	// - for "probabilistic" sampler, a probability between 0 and 1
	// - for "rateLimiting" sampler, the number of spans per second
	// - for "remote" sampler, param is the same as for "probabilistic"
	//   and indicates the initial sampling rate before the actual one
	//   is received from the mothership
	Param float64 `yaml:"param"`

	// LocalAgentHostPort is the address of jaeger-agent's HTTP server
	LocalAgentHostPort string `yaml:"localAgentHostPort"`
}

// ReporterConfig configures the reporter. All fields are optional.
type ReporterConfig struct {
	// QueueSize controls how many spans the reporter can keep in memory before it starts dropping
	// new spans. The queue is continuously drained by a background go-routine, as fast as spans
	// can be sent out of process.
	QueueSize int `yaml:"queueSize"`

	// BufferFlushInterval controls how often the buffer is force-flushed, even if it's not full.
	// It is generally not useful, as it only matters for very low traffic services.
	BufferFlushInterval time.Duration

	// LogSpans, when true, enables LoggingReporter that runs in parallel with the main reporter
	// and logs all submitted spans. Main Configuration.Logger must be initialized in the code
	// for this option to have any effect.
	LogSpans bool `yaml:"logSpans"`

	// LocalAgentHostPort instructs reporter to send spans to jaeger-agent at this address
	LocalAgentHostPort string `yaml:"localAgentHostPort"`
}

type nullCloser struct{}

func (*nullCloser) Close() error { return nil }

// New creates a new Jaeger Tracer, and a closer func that can be used to flush buffers
// before shutdown.
func (c Configuration) New(
	serviceName string,
	statsReporter jaeger.StatsReporter,
) (opentracing.Tracer, io.Closer, error) {
	if c.Disabled {
		return &opentracing.NoopTracer{}, &nullCloser{}, nil
	}
	if serviceName == "" {
		return nil, nil, errors.New("no service name provided")
	}
	if c.Sampler == nil {
		c.Sampler = &SamplerConfig{Type: samplerTypeProbabilistic, Param: 0.01}
	}
	if c.Reporter == nil {
		c.Reporter = &ReporterConfig{}
	}

	metrics := jaeger.NewMetrics(statsReporter, nil)

	sampler, err := c.Sampler.NewSampler(serviceName, metrics)
	if err != nil {
		return nil, nil, err
	}

	reporter, err := c.Reporter.NewReporter(serviceName, metrics, c.Logger)
	if err != nil {
		return nil, nil, err
	}

	tracer, closer := jaeger.NewTracer(
		serviceName,
		sampler,
		reporter,
		jaeger.TracerOptions.Metrics(metrics),
		jaeger.TracerOptions.Logger(c.Logger))

	return tracer, closer, nil
}

// InitGlobalTracer creates a new Jaeger Tracer, and sets is as global OpenTracing Tracer.
// It returns a closer func that can be used to flush buffers before shutdown.
func (c Configuration) InitGlobalTracer(
	serviceName string,
	statsReporter jaeger.StatsReporter,
) (io.Closer, error) {
	if c.Disabled {
		return &nullCloser{}, nil
	}
	tracer, closer, err := c.New(serviceName, statsReporter)
	if err != nil {
		return nil, err
	}
	opentracing.InitGlobalTracer(tracer)
	return closer, nil
}

const (
	samplerTypeConst         = "const"
	samplerTypeRemote        = "remote"
	samplerTypeProbabilistic = "probabilistic"
	samplerTypeRateLimiting  = "rateLimiting"
)

// NewSampler creates a new sampler based on the configuration
func (sc *SamplerConfig) NewSampler(
	serviceName string,
	metrics *jaeger.Metrics,
) (jaeger.Sampler, error) {
	if sc.Type == samplerTypeConst {
		return jaeger.NewConstSampler(sc.Param != 0), nil
	}
	if sc.Type == samplerTypeProbabilistic {
		if sc.Param >= 0 && sc.Param <= 1.0 {
			return jaeger.NewProbabilisticSampler(sc.Param)
		}
		return nil, fmt.Errorf(
			"Invalid Param for probabilistic sampler: %v. Expecting value between 0 and 1",
			sc.Param,
		)
	}
	if sc.Type == samplerTypeRateLimiting {
		return jaeger.NewRateLimitingSampler(sc.Param)
	}
	if sc.Type == samplerTypeRemote || sc.Type == "" {
		sc2 := *sc
		sc2.Type = samplerTypeProbabilistic
		initSampler, err := sc2.NewSampler(serviceName, nil)
		if err != nil {
			return nil, err
		}
		return jaeger.NewRemotelyControlledSampler(serviceName, initSampler,
			sc.LocalAgentHostPort, metrics, jaeger.NullLogger), nil
	}
	return nil, fmt.Errorf("Unknown sampler type %v", sc.Type)
}

// NewReporter instantiates a new reporter that submits spans to tcollector
func (rc *ReporterConfig) NewReporter(
	serviceName string,
	metrics *jaeger.Metrics,
	logger jaeger.Logger,
) (jaeger.Reporter, error) {
	sender, err := rc.newTransport()
	if err != nil {
		return nil, err
	}

	reporter := jaeger.NewRemoteReporter(
		sender,
		&jaeger.ReporterOptions{
			QueueSize:           rc.QueueSize,
			BufferFlushInterval: rc.BufferFlushInterval,
			Logger:              logger,
			Metrics:             metrics})
	if rc.LogSpans && logger != nil {
		logger.Infof("Initializing logging reporter\n")
		reporter = jaeger.NewCompositeReporter(jaeger.NewLoggingReporter(logger), reporter)
	}
	return reporter, err
}

func (rc *ReporterConfig) newTransport() (transport.Transport, error) {
	return udp.NewUDPTransport(rc.LocalAgentHostPort, 0)
}
