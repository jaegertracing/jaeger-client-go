package zipkin

import (
	"strconv"
	"strings"

	opentracing "github.com/opentracing/opentracing-go"

	"github.com/uber/jaeger-client-go"
)

// Propagator is an Injector and Extractor
type Propagator struct{}

// NewZipkinB3HTTPHeaderPropagator creates a Propagator for extracting and injecting
// Zipkin HTTP B3 headers into SpanContexts.
func NewZipkinB3HTTPHeaderPropagator() Propagator {
	return Propagator{}
}

// Inject conforms to the Injector interface for decoding Zipkin HTTP B3 headers
func (p Propagator) Inject(
	sc jaeger.SpanContext,
	abstractCarrier interface{},
) error {
	textMapWriter, ok := abstractCarrier.(opentracing.TextMapWriter)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}

	textMapWriter.Set("x-b3-traceid", strconv.FormatUint(sc.TraceID(), 16))
	if sc.ParentID() != 0 {
		textMapWriter.Set("x-b3-parentspanid", strconv.FormatUint(sc.ParentID(), 16))
	}
	textMapWriter.Set("x-b3-spanid", strconv.FormatUint(sc.SpanID(), 16))
	if sc.IsSampled() {
		textMapWriter.Set("x-b3-sampled", "1")
	} else {
		textMapWriter.Set("x-b3-sampled", "0")
	}
	return nil
}

// Extract conforms to the Extractor interface for encoding Zipkin HTTP B3 headers
func (p Propagator) Extract(abstractCarrier interface{}) (jaeger.SpanContext, error) {
	textMapReader, ok := abstractCarrier.(opentracing.TextMapReader)
	if !ok {
		return jaeger.SpanContext{}, opentracing.ErrInvalidCarrier
	}
	var traceID uint64
	var spanID uint64
	var parentID uint64
	sampled := false
	err := textMapReader.ForeachKey(func(rawKey, value string) error {
		key := strings.ToLower(rawKey) // TODO not necessary for plain TextMap
		var err error
		if key == "x-b3-traceid" {
			traceID, err = strconv.ParseUint(value, 16, 64)
		} else if key == "x-b3-parentspanid" {
			parentID, err = strconv.ParseUint(value, 16, 64)
		} else if key == "x-b3-spanid" {
			spanID, err = strconv.ParseUint(value, 16, 64)
		} else if key == "x-b3-sampled" && value == "1" {
			sampled = true
		}
		return err
	})

	if err != nil {
		return jaeger.SpanContext{}, err
	}
	if traceID == 0 {
		return jaeger.SpanContext{}, opentracing.ErrSpanContextNotFound
	}
	return jaeger.NewSpanContext(traceID, spanID, parentID, sampled, nil), nil
}
