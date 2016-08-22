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
	"fmt"
	"io"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/uber/jaeger-client-go/utils"
)

type tracer struct {
	serviceName string
	hostIPv4    uint32

	sampler  Sampler
	reporter Reporter
	metrics  Metrics
	logger   Logger

	timeNow      func() time.Time
	randomNumber func() uint64

	// pool for Span objects
	poolSpans bool
	spanPool  sync.Pool

	injectors  map[interface{}]Injector
	extractors map[interface{}]Extractor

	tags []tag
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

	httpHeaderPropagator := newHTTPHeaderPropagator(t)
	t.injectors[opentracing.HTTPHeaders] = httpHeaderPropagator
	t.extractors[opentracing.HTTPHeaders] = httpHeaderPropagator

	binaryPropagator := newBinaryPropagator(t)
	t.injectors[opentracing.Binary] = binaryPropagator
	t.extractors[opentracing.Binary] = binaryPropagator

	// TODO remove after TChannel supports OpenTracing
	interopPropagator := &jaegerTraceContextPropagator{tracer: t}
	t.injectors[SpanContextFormat] = interopPropagator
	t.extractors[SpanContextFormat] = interopPropagator

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
	if t.logger == nil {
		t.logger = NullLogger
	}
	// TODO once on the new data model, support both v4 and v6 IPs
	if t.hostIPv4 == 0 {
		if ip, err := utils.HostIP(); err == nil {
			t.hostIPv4 = utils.PackIPAsUint32(ip)
		} else {
			t.logger.Error("Unable to determine this host's IP address: " + err.Error())
		}
	}
	// Set tracer-level tags
	t.tags = append(t.tags, tag{key: JaegerClientTag, value: JaegerGoVersion})
	if hostname, err := os.Hostname(); err == nil {
		t.tags = append(t.tags, tag{key: TracerHostnameKey, value: hostname})
	}

	return t, t
}

func (t *tracer) StartSpan(
	operationName string,
	options ...opentracing.StartSpanOption,
) opentracing.Span {
	sso := opentracing.StartSpanOptions{}
	for _, o := range options {
		o.Apply(&sso)
	}
	return t.startSpanWithOptions(operationName, sso)
}

func (t *tracer) startSpanWithOptions(
	operationName string,
	options opentracing.StartSpanOptions,
) opentracing.Span {
	startTime := options.StartTime
	if startTime.IsZero() {
		startTime = t.timeNow()
	}

	var parent SpanContext
	var hasParent bool
	for _, ref := range options.References {
		if ref.Type == opentracing.ChildOfRef {
			if p, ok := ref.ReferencedContext.(SpanContext); ok {
				parent = p
				hasParent = true
				break
			} else {
				t.logger.Error(fmt.Sprintf(
					"ChildOf reference contains invalid type of SpanReference: %s",
					reflect.ValueOf(ref.ReferencedContext)))
			}
		} else {
			// TODO support other types of span references
		}
	}

	rpcServer := false
	if v, ok := options.Tags[ext.SpanKindRPCServer.Key]; ok {
		rpcServer = (v == ext.SpanKindRPCServerEnum || v == string(ext.SpanKindRPCServerEnum))
	}

	var ctx SpanContext
	if !hasParent {
		ctx.traceID = t.randomID()
		ctx.spanID = ctx.traceID
		ctx.parentID = 0
		ctx.flags = byte(0)
		if t.sampler.IsSampled(ctx.traceID) {
			ctx.flags |= flagSampled
		}
	} else {
		ctx.traceID = parent.traceID
		if rpcServer {
			// Support Zipkin's one-span-per-RPC model
			ctx.spanID = parent.spanID
			ctx.parentID = parent.parentID
		} else {
			ctx.spanID = t.randomID()
			ctx.parentID = parent.spanID
		}
		ctx.flags = parent.flags
		// copy baggage items
		if l := len(parent.baggage); l > 0 {
			ctx.baggage = make(map[string]string, len(parent.baggage))
			for k, v := range parent.baggage {
				ctx.baggage[k] = v
			}
		}
	}

	sp := t.newSpan()
	sp.context = ctx
	return t.startSpanInternal(
		sp,
		operationName,
		startTime,
		options.Tags,
		rpcServer, /* joining with external trace if rpcServer */
	)
}

// Inject implements Inject() method of opentracing.Tracer
func (t *tracer) Inject(ctx opentracing.SpanContext, format interface{}, carrier interface{}) error {
	c, ok := ctx.(SpanContext)
	if !ok {
		return opentracing.ErrInvalidSpanContext
	}
	if injector, ok := t.injectors[format]; ok {
		return injector.Inject(c, carrier)
	}
	return opentracing.ErrUnsupportedFormat
}

// Extract implements Extract() method of opentracing.Tracer
func (t *tracer) Extract(
	format interface{},
	carrier interface{},
) (opentracing.SpanContext, error) {
	if extractor, ok := t.extractors[format]; ok {
		return extractor.Extract(carrier)
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
	sp.context = emptyContext
	sp.tracer = nil
	sp.tags = nil
	sp.logs = nil
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
	sp.firstInProcess = join || sp.context.parentID == 0
	if tags != nil && len(tags) > 0 {
		sp.tags = make([]tag, 0, len(tags))
		for k, v := range tags {
			if k == string(ext.SamplingPriority) && setSamplingPriority(sp, k, v) {
				continue
			}
			sp.setTagNoLocking(k, v)
		}
	}
	// emit metrics
	t.metrics.SpansStarted.Inc(1)
	if sp.context.IsSampled() {
		t.metrics.SpansSampled.Inc(1)
		if join {
			t.metrics.TracesJoinedSampled.Inc(1)
		} else if sp.context.parentID == 0 {
			t.metrics.TracesStartedSampled.Inc(1)
		}
	} else {
		t.metrics.SpansNotSampled.Inc(1)
		if join {
			t.metrics.TracesJoinedNotSampled.Inc(1)
		} else if sp.context.parentID == 0 {
			t.metrics.TracesStartedNotSampled.Inc(1)
		}

	}
	return sp
}

func (t *tracer) reportSpan(sp *span) {
	t.metrics.SpansFinished.Inc(1)
	if sp.context.IsSampled() {
		if sp.firstInProcess {
			// TODO when migrating to new data model, this can be optimized
			sp.setTracerTags(t.tags)
		}
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
