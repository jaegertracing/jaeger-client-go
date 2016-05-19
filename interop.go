package jaeger

import (
	"github.com/opentracing/opentracing-go"
	"strings"
)

type formatKey int

// TraceContextFormat is a constant used as OpenTracing Format. Requires TraceContextCarrier.
// This format is intended for interop with TChannel or other Zipkin-like tracers.
const TraceContextFormat formatKey = iota

// TraceContextCarrier is a carrier type used with TraceContextFormat.
type TraceContextCarrier struct {
	TraceContext TraceContext
	Baggage      map[string]string
}

type jaegerTraceContextPropagator struct {
	tracer *tracer
}

func (p *jaegerTraceContextPropagator) InjectSpan(
	sp opentracing.Span,
	abstractCarrier interface{},
) error {
	sc, ok := sp.(*span)
	if !ok {
		return opentracing.ErrInvalidSpan
	}
	carrier, ok := abstractCarrier.(*TraceContextCarrier)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}

	sc.RLock()
	defer sc.RUnlock()

	carrier.TraceContext = sc.TraceContext

	if l := len(sc.baggage); l > 0 && carrier.Baggage == nil {
		carrier.Baggage = make(map[string]string, l)
	}
	for k, v := range sc.baggage {
		safeKey := encodeBaggageKeyAsHeader(k)
		carrier.Baggage[safeKey] = v
	}
	return nil
}

func (p *jaegerTraceContextPropagator) Join(
	operationName string,
	abstractCarrier interface{},
) (opentracing.Span, error) {
	carrier, ok := abstractCarrier.(*TraceContextCarrier)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	context := carrier.TraceContext
	if context.traceID == 0 {
		return nil, opentracing.ErrTraceNotFound
	}
	var baggage map[string]string
	if l := len(carrier.Baggage); l > 0 {
		baggage = make(map[string]string, l)
		for k, v := range carrier.Baggage {
			lowerCaseKey := strings.ToLower(k)
			if strings.HasPrefix(lowerCaseKey, TraceBaggageHeaderPrefix) {
				key := decodeBaggageHeaderKey(lowerCaseKey)
				baggage[key] = v
			}
		}
	}
	sp := p.tracer.newSpan()
	sp.TraceContext = context
	sp.baggage = baggage
	return p.tracer.startSpanInternal(
		sp,
		operationName,
		p.tracer.timeNow(),
		nil,
		true, // join with external trace
	), nil
}
