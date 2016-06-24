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
	ext.SpanKind.Set(sp, ext.SpanKindRPCServer)
	ext.PeerService.Set(sp, "peer-service")
	s.NotNil(sp)
	ss := sp.(*span)
	s.NotNil(ss.tracer, "Tracer must be referenced from span")
	s.Equal("get_name", ss.operationName)
	s.Equal("s", ss.spanKind, "Span must be server-side")
	s.Equal("peer-service", ss.peer.ServiceName, "Client is 'peer-service'")

	s.EqualValues(someID, ss.traceID)
	s.EqualValues(0, ss.parentID)

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

func (s *tracerSuite) TestBeginRootSpanWithOptions() {
	ts := time.Now()
	options := opentracing.StartSpanOptions{
		OperationName: "get_address",
		StartTime:     ts,
	}
	sp := s.tracer.StartSpanWithOptions(options)
	ss := sp.(*span)
	s.Equal("get_address", ss.operationName)
	s.Equal("", ss.spanKind, "Span must not be RPC")
	s.Equal(ts, ss.startTime)
}

func (s *tracerSuite) TestBeginChildSpan() {
	sp1 := s.tracer.StartSpan("get_address")
	sp2 := opentracing.StartChildSpan(sp1, "get_street")
	s.Equal(sp1.(*span).spanID, sp2.(*span).parentID)
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

func (s *tracerSuite) TestSetOperationName() {
	sp1 := s.tracer.StartSpan("get_address")
	sp1.SetOperationName("get_street")
	s.Equal("get_street", sp1.(*span).operationName)
}

func (s *tracerSuite) TestSamplerEffects() {
	s.tracer.(*tracer).sampler = NewConstSampler(true)
	sp := s.tracer.StartSpan("test")
	flags := sp.(*span).flags
	s.EqualValues(flagSampled, flags&flagSampled)

	s.tracer.(*tracer).sampler = NewConstSampler(false)
	sp = s.tracer.StartSpan("test")
	flags = sp.(*span).flags
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
	s.EqualValues(int64(1), sp.traceID)

	rng := utils.NewRand(0)
	rng.Seed(1) // for test coverage
}

func TestInjectorExtractorOptions(t *testing.T) {
	tracer, tc := NewTracer("x", NewConstSampler(true), NewNullReporter(),
		TracerOptions.Injector("dummy", &dummyPropagator{}),
		TracerOptions.Extractor("dummy", &dummyPropagator{}),
	)
	defer tc.Close()

	sp := tracer.StartSpan("x")
	c := &dummyCarrier{}
	err := tracer.Inject(sp, "dummy", []int{})
	assert.Equal(t, opentracing.ErrInvalidCarrier, err)
	err = tracer.Inject(sp, "dummy", c)
	assert.NoError(t, err)
	assert.True(t, c.ok)

	c.ok = false
	_, err = tracer.Join("z", "dummy", []int{})
	assert.Equal(t, opentracing.ErrInvalidCarrier, err)
	_, err = tracer.Join("z", "dummy", c)
	assert.Equal(t, opentracing.ErrTraceNotFound, err)
	c.ok = true
	_, err = tracer.Join("z", "dummy", c)
	assert.NoError(t, err)
}

type dummyPropagator struct{}
type dummyCarrier struct {
	ok bool
}

func (p *dummyPropagator) InjectSpan(span opentracing.Span, carrier interface{}) error {
	c, ok := carrier.(*dummyCarrier)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}
	c.ok = true
	return nil
}

func (p *dummyPropagator) Join(operationName string, carrier interface{}) (opentracing.Span, error) {
	c, ok := carrier.(*dummyCarrier)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	if c.ok {
		return nil, nil
	}
	return nil, opentracing.ErrTraceNotFound
}
