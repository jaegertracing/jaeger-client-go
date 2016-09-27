// Copyright (c) 2015 Uber Technologies, Inc.

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
	"io"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/uber/jaeger-client-go/utils"
)

type tracerSuite struct {
	suite.Suite
	tracer opentracing.Tracer
	closer io.Closer
	stats  *InMemoryStatsCollector
}

var IP uint32 = 1<<24 | 2<<16 | 3<<8 | 4

func (s *tracerSuite) SetupTest() {
	s.stats = NewInMemoryStatsCollector()
	metrics := NewMetrics(s.stats, nil)

	s.tracer, s.closer = NewTracer("DOOP", // respect the classics, man!
		NewConstSampler(true),
		NewNullReporter(),
		TracerOptions.Metrics(metrics),
		TracerOptions.HostIPv4(IP))
	s.NotNil(s.tracer)
}

func (s *tracerSuite) TearDownTest() {
	if s.tracer != nil {
		s.closer.Close()
		s.tracer = nil
	}
}

func TestTracerSuite(t *testing.T) {
	suite.Run(t, new(tracerSuite))
}

func (s *tracerSuite) TestBeginRootSpan() {
	startTime := time.Now()
	s.tracer.(*tracer).timeNow = func() time.Time { return startTime }
	someID := uint64(12345)
	s.tracer.(*tracer).randomNumber = func() uint64 { return someID }

	sp := s.tracer.StartSpan("get_name")
	ext.SpanKindRPCServer.Set(sp)
	ext.PeerService.Set(sp, "peer-service")
	s.NotNil(sp)
	ss := sp.(*span)
	s.NotNil(ss.tracer, "Tracer must be referenced from span")
	s.Equal("get_name", ss.operationName)
	s.Equal("server", ss.spanKind, "Span must be server-side")
	s.Equal("peer-service", ss.peer.ServiceName, "Client is 'peer-service'")

	s.EqualValues(someID, ss.context.traceID)
	s.EqualValues(0, ss.context.parentID)

	s.Equal(startTime, ss.startTime)

	sp.Finish()
	s.NotNil(ss.duration)

	s.EqualValues(map[string]int64{
		"jaeger.spans|group=lifecycle|state=started":  1,
		"jaeger.spans|group=lifecycle|state=finished": 1,
		"jaeger.spans|group=sampling|sampled=y":       1,
		"jaeger.traces|sampled=y|state=started":       1,
	}, s.stats.GetCounterValues())
}

func (s *tracerSuite) TestStartRootSpanWithOptions() {
	ts := time.Now()
	sp := s.tracer.StartSpan("get_address", opentracing.StartTime(ts))
	ss := sp.(*span)
	s.Equal("get_address", ss.operationName)
	s.Equal("", ss.spanKind, "Span must not be RPC")
	s.Equal(ts, ss.startTime)
}

func (s *tracerSuite) TestStartChildSpan() {
	sp1 := s.tracer.StartSpan("get_address")
	sp2 := s.tracer.StartSpan("get_street", opentracing.ChildOf(sp1.Context()))
	s.Equal(sp1.(*span).context.spanID, sp2.(*span).context.parentID)
	sp2.Finish()
	s.NotNil(sp2.(*span).duration)
	sp1.Finish()
	s.EqualValues(map[string]int64{
		"jaeger.spans|group=sampling|sampled=y":       2,
		"jaeger.traces|sampled=y|state=started":       1,
		"jaeger.spans|group=lifecycle|state=started":  2,
		"jaeger.spans|group=lifecycle|state=finished": 2,
	}, s.stats.GetCounterValues())
}

func (s *tracerSuite) TestStartRPCServerSpan() {
	sp1 := s.tracer.StartSpan("get_address")
	sp2 := s.tracer.StartSpan("get_street", ext.RPCServerOption(sp1.Context()))
	s.Equal(sp1.(*span).context.spanID, sp2.(*span).context.spanID)
	s.Equal(sp1.(*span).context.parentID, sp2.(*span).context.parentID)
	sp2.Finish()
	s.NotNil(sp2.(*span).duration)
	sp1.Finish()
	s.EqualValues(map[string]int64{
		"jaeger.spans|group=sampling|sampled=y":       2,
		"jaeger.traces|sampled=y|state=started":       1,
		"jaeger.traces|sampled=y|state=joined":        1,
		"jaeger.spans|group=lifecycle|state=started":  2,
		"jaeger.spans|group=lifecycle|state=finished": 2,
	}, s.stats.GetCounterValues())
}

func (s *tracerSuite) TestSetOperationName() {
	sp1 := s.tracer.StartSpan("get_address")
	sp1.SetOperationName("get_street")
	s.Equal("get_street", sp1.(*span).operationName)
}

func (s *tracerSuite) TestSamplerEffects() {
	s.tracer.(*tracer).sampler = NewConstSampler(true)
	sp := s.tracer.StartSpan("test")
	flags := sp.(*span).context.flags
	s.EqualValues(flagSampled, flags&flagSampled)

	s.tracer.(*tracer).sampler = NewConstSampler(false)
	sp = s.tracer.StartSpan("test")
	flags = sp.(*span).context.flags
	s.EqualValues(0, flags&flagSampled)
}

func (s *tracerSuite) TestRandomIDNotZero() {
	val := uint64(0)
	s.tracer.(*tracer).randomNumber = func() (r uint64) {
		r = val
		val++
		return
	}
	sp := s.tracer.StartSpan("get_name").(*span)
	s.EqualValues(int64(1), sp.context.traceID)

	rng := utils.NewRand(0)
	rng.Seed(1) // for test coverage
}

func TestTracerOptions(t *testing.T) {
	t1, e := time.Parse(time.RFC3339, "2012-11-01T22:08:41+00:00")
	assert.NoError(t, e)

	timeNow := func() time.Time {
		return t1
	}
	rnd := func() uint64 {
		return 1
	}

	openTracer, closer := NewTracer("DOOP", // respect the classics, man!
		NewConstSampler(true),
		NewNullReporter(),
		TracerOptions.Logger(StdLogger),
		TracerOptions.TimeNow(timeNow),
		TracerOptions.RandomNumber(rnd),
		TracerOptions.PoolSpans(true),
	)
	defer closer.Close()

	tracer := openTracer.(*tracer)
	assert.Equal(t, StdLogger, tracer.logger)
	assert.Equal(t, t1, tracer.timeNow())
	assert.Equal(t, uint64(1), tracer.randomNumber())
	assert.Equal(t, uint64(1), tracer.randomNumber())
	assert.Equal(t, uint64(1), tracer.randomNumber()) // always 1
	assert.Equal(t, true, tracer.poolSpans)
}

func TestInjectorExtractorOptions(t *testing.T) {
	tracer, tc := NewTracer("x", NewConstSampler(true), NewNullReporter(),
		TracerOptions.Injector("dummy", &dummyPropagator{}),
		TracerOptions.Extractor("dummy", &dummyPropagator{}),
	)
	defer tc.Close()

	sp := tracer.StartSpan("x")
	c := &dummyCarrier{}
	err := tracer.Inject(sp.Context(), "dummy", []int{})
	assert.Equal(t, opentracing.ErrInvalidCarrier, err)
	err = tracer.Inject(sp.Context(), "dummy", c)
	assert.NoError(t, err)
	assert.True(t, c.ok)

	c.ok = false
	_, err = tracer.Extract("dummy", []int{})
	assert.Equal(t, opentracing.ErrInvalidCarrier, err)
	_, err = tracer.Extract("dummy", c)
	assert.Equal(t, opentracing.ErrSpanContextNotFound, err)
	c.ok = true
	_, err = tracer.Extract("dummy", c)
	assert.NoError(t, err)
}

func TestEmptySpanContextAsParent(t *testing.T) {
	tracer, tc := NewTracer("x", NewConstSampler(true), NewNullReporter())
	defer tc.Close()

	span := tracer.StartSpan("test", opentracing.ChildOf(emptyContext))
	ctx := span.Context().(SpanContext)
	assert.NotEqual(t, uint64(0), uint64(ctx.traceID))
	assert.True(t, ctx.IsValid())
}

type dummyPropagator struct{}
type dummyCarrier struct {
	ok bool
}

func (p *dummyPropagator) Inject(ctx SpanContext, carrier interface{}) error {
	c, ok := carrier.(*dummyCarrier)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}
	c.ok = true
	return nil
}

func (p *dummyPropagator) Extract(carrier interface{}) (SpanContext, error) {
	c, ok := carrier.(*dummyCarrier)
	if !ok {
		return emptyContext, opentracing.ErrInvalidCarrier
	}
	if c.ok {
		return emptyContext, nil
	}
	return emptyContext, opentracing.ErrSpanContextNotFound
}
