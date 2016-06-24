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

package jaeger

import (
	"io"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"

	"github.com/uber/jaeger-client-go/utils"
)

type tracer struct {
	serviceName string
	hostIPv4    uint32

	sampler  Sampler
	reporter Reporter
	metrics  Metrics

	timeNow      func() time.Time
	randomNumber func() uint64

	// pool for Span objects
	poolSpans bool
	spanPool  sync.Pool

	injectors  map[interface{}]Injector
	extractors map[interface{}]Extractor
}

// NewTracer creates Tracer implementation that reports tracing to Jaeger.
// The returned io.Closer can be used in shutdown hooks to ensure that the internal
// queue of the Reporter is drained and all buffered spans are submitted to collectors.
func NewTracer(
	serviceName string,
	sampler Sampler,
	reporter Reporter,
	options ...TracerOption,
) (opentracing.Tracer, io.Closer) {
	t := &tracer{
		serviceName: serviceName,
		sampler:     sampler,
		reporter:    reporter,
		injectors:   make(map[interface{}]Injector),
		extractors:  make(map[interface{}]Extractor),
		metrics:     *NewMetrics(NullStatsReporter, nil),
		spanPool: sync.Pool{New: func() interface{} {
			return &span{}
		}},
	}

	// register default injectors/extractors
	textPropagator := newTextMapPropagator(t)
	t.injectors[opentracing.TextMap] = textPropagator
	t.extractors[opentracing.TextMap] = textPropagator

	binaryPropagator := newBinaryPropagator(t)
	t.injectors[opentracing.Binary] = binaryPropagator
	t.extractors[opentracing.Binary] = binaryPropagator

	interopPropagator := &jaegerTraceContextPropagator{tracer: t}
	t.injectors[TraceContextFormat] = interopPropagator
	t.extractors[TraceContextFormat] = interopPropagator

	zipkinPropagator := &zipkinPropagator{tracer: t}
	t.injectors[ZipkinSpanFormat] = zipkinPropagator
	t.extractors[ZipkinSpanFormat] = zipkinPropagator

	for _, option := range options {
		option(t)
	}

	if t.randomNumber == nil {
		rng := utils.NewRand(time.Now().UnixNano())
		t.randomNumber = func() uint64 { return uint64(rng.Int63()) }
	}
	if t.timeNow == nil {
		t.timeNow = time.Now
	}
	if t.hostIPv4 == 0 {
		var localIPInt32 uint32
		if localIP := utils.GetLocalIP(); localIP != nil {
			localIPInt32, _ = utils.IPToUint32(localIP.String())
		}
		t.hostIPv4 = localIPInt32
	}

	return t, t
}

func (t *tracer) StartSpan(operationName string) opentracing.Span {
	return t.StartSpanWithOptions(
		opentracing.StartSpanOptions{
			OperationName: operationName,
		})
}

func (t *tracer) StartSpanWithOptions(options opentracing.StartSpanOptions) opentracing.Span {
	startTime := options.StartTime
	if startTime.IsZero() {
		startTime = t.timeNow()
	}

	sp := t.newSpan()
	if options.Parent == nil {
		sp.traceID = t.randomID()
		sp.spanID = sp.traceID
		sp.parentID = 0
		sp.flags = byte(0)
		if t.sampler.IsSampled(sp.traceID) {
			sp.flags |= flagSampled
		}
	} else {
		parent := options.Parent.(*span)
		parent.RLock()
		sp.traceID = parent.traceID
		sp.spanID = t.randomID()
		sp.parentID = parent.spanID
		sp.flags = parent.flags
		// copy baggage items
		if l := len(parent.baggage); l > 0 {
			sp.baggage = make(map[string]string, len(parent.baggage))
			for k, v := range parent.baggage {
				sp.baggage[k] = v
			}
		}
		parent.RUnlock()
	}

	return t.startSpanInternal(
		sp,
		options.OperationName,
		startTime,
		options.Tags,
		false, /* not a join with external trace */
	)
}

// Inject implements Inject() method of opentracing.Tracer
func (t *tracer) Inject(sp opentracing.Span, format interface{}, carrier interface{}) error {
	if injector, ok := t.injectors[format]; ok {
		return injector.InjectSpan(sp, carrier)
	}
	return opentracing.ErrUnsupportedFormat
}

// Join implements Join() method of opentracing.Tracer
func (t *tracer) Join(
	operationName string,
	format interface{},
	carrier interface{},
) (opentracing.Span, error) {
	if extractor, ok := t.extractors[format]; ok {
		return extractor.Join(operationName, carrier)
	}
	return nil, opentracing.ErrUnsupportedFormat
}

// Close releases all resources used by the Tracer and flushes any remaining buffered spans.
func (t *tracer) Close() error {
	t.reporter.Close()
	t.sampler.Close()
	return nil
}

// getSpan retrieves an instance of a clean Span object.
// If options.PoolSpans is true, the spans are retrieved from an object pool.
func (t *tracer) newSpan() *span {
	if !t.poolSpans {
		return &span{}
	}
	sp := t.spanPool.Get().(*span)
	sp.tracer = nil
	sp.tags = nil
	sp.logs = nil
	sp.baggage = nil
	return sp
}

func (t *tracer) startSpanInternal(
	sp *span,
	operationName string,
	startTime time.Time,
	tags opentracing.Tags,
	join bool, // are we joining an external trace?
) opentracing.Span {
	sp.tracer = t
	sp.operationName = operationName
	sp.startTime = startTime
	sp.duration = 0
	sp.firstInProcess = join || sp.parentID == 0
	if tags != nil && len(tags) > 0 {
		sp.tags = make([]tag, 0, len(tags))
		for k, v := range tags {
			sp.tags = append(sp.tags, tag{key: k, value: v})
		}
	}
	// emit metrics
	t.metrics.SpansStarted.Inc(1)
	if sp.IsSampled() {
		t.metrics.SpansSampled.Inc(1)
		if sp.parentID == 0 {
			t.metrics.TracesStartedSampled.Inc(1)
		} else if join {
			t.metrics.TracesJoinedSampled.Inc(1)
		}
	} else {
		t.metrics.SpansNotSampled.Inc(1)
		if sp.parentID == 0 {
			t.metrics.TracesStartedNotSampled.Inc(1)
		} else if join {
			t.metrics.TracesJoinedNotSampled.Inc(1)
		}
	}
	return sp
}

func (t *tracer) reportSpan(sp *span) {
	t.metrics.SpansFinished.Inc(1)
	if sp.IsSampled() {
		t.reporter.Report(sp)
	}
	if t.poolSpans {
		t.spanPool.Put(sp)
	}
}

// randomID generates a random trace/span ID, using tracer.random() generator.
// It never returns 0.
func (t *tracer) randomID() uint64 {
	val := t.randomNumber()
	for val == 0 {
		val = t.randomNumber()
	}
	return val
}
