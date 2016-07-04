package jaeger

import (
	"github.com/opentracing/opentracing-go"
)

// ZipkinSpanFormat is an OpenTracing carrier format constant for ZipkinSpan carrier type below.
const ZipkinSpanFormat = "zipkin-span-format"

// BaggageOnlyTextMap is a variation of TextMap OpenTracing format which only stores baggage.
const BaggageOnlyTextMap = "baggage-only-textmap"

// ZipkinSpan is a type of Carrier used for integration with Zipkin-aware RPC frameworks
// (like TChannel). It does not support baggage, only trace IDs.
type ZipkinSpan interface {
	TraceID() uint64
	SpanID() uint64
	ParentID() uint64
	Flags() byte
	SetTraceID(traceID uint64)
	SetSpanID(spanID uint64)
	SetParentID(parentID uint64)
	SetFlags(flags byte)
}

type zipkinPropagator struct {
	tracer *tracer
}

func (p *zipkinPropagator) InjectSpan(
	sp opentracing.Span,
	abstractCarrier interface{},
) error {
	sc, ok := sp.(*span)
	if !ok {
		return opentracing.ErrInvalidSpan
	}
	carrier, ok := abstractCarrier.(ZipkinSpan)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}

	sc.RLock()
	defer sc.RUnlock()

	carrier.SetTraceID(sc.TraceContext.TraceID())
	carrier.SetSpanID(sc.TraceContext.SpanID())
	carrier.SetParentID(sc.TraceContext.ParentID())
	carrier.SetFlags(sc.TraceContext.flags)

	return nil
}

func (p *zipkinPropagator) Join(
	operationName string,
	abstractCarrier interface{},
) (opentracing.Span, error) {
	carrier, ok := abstractCarrier.(ZipkinSpan)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	if carrier.TraceID() == 0 {
		return nil, opentracing.ErrTraceNotFound
	}
	sp := p.tracer.newSpan()
	sp.TraceContext.traceID = carrier.TraceID()
	sp.TraceContext.spanID = carrier.SpanID()
	sp.TraceContext.parentID = carrier.ParentID()
	sp.TraceContext.flags = carrier.Flags()

	return p.tracer.startSpanInternal(
		sp,
		operationName,
		p.tracer.timeNow(),
		nil,
		true, // join with external trace
	), nil
}

type baggageOnlyTextMapPropagator struct {
	tracer *tracer
}

func (p *baggageOnlyTextMapPropagator) InjectSpan(
	sp opentracing.Span,
	abstractCarrier interface{},
) error {
	sc, ok := sp.(*span)
	if !ok {
		return opentracing.ErrInvalidSpan
	}
	carrier, ok := abstractCarrier.(opentracing.TextMapWriter)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}

	sc.RLock()
	defer sc.RUnlock()

	for k, v := range sc.baggage {
		carrier.Set(k, v)
	}

	return nil
}
