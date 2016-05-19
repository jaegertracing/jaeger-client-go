package jaeger

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"sync"

	opentracing "github.com/opentracing/opentracing-go"
)

// Injector is responsible for injecting Span instances in a manner suitable
// for propagation via a format-specific "carrier" object. Typically the
// injection will take place across an RPC boundary, but message queues and
// other IPC mechanisms are also reasonable places to use an Injector.
type Injector interface {
	// InjectSpan takes `span` and injects it into `carrier`. The actual type
	// of `carrier` depends on the `format` passed to `Tracer.Injector()`.
	//
	// Implementations may return opentracing.ErrInvalidCarrier or any other
	// implementation-specific error if injection fails.
	InjectSpan(span opentracing.Span, carrier interface{}) error
}

// Extractor is responsible for extracting Span instances from an
// format-specific "carrier" object. Typically the extraction will take place
// on the server side of an RPC boundary, but message queues and other IPC
// mechanisms are also reasonable places to use an Extractor.
type Extractor interface {
	// Join returns a Span instance with operation name `operationName`
	// given `carrier`, or (nil, opentracing.ErrTraceNotFound) if no trace could be found to
	// join with in the `carrier`.
	//
	// Implementations may return opentracing.ErrInvalidCarrier,
	// opentracing.ErrTraceCorrupted, or implementation-specific errors if there
	// are more fundamental problems with `carrier`.
	//
	// Upon success, the returned Span instance is already started.
	Join(operationName string, carrier interface{}) (opentracing.Span, error)
}

type textMapPropagator struct {
	tracer *tracer
}

func newTextMapPropagator(tracer *tracer) *textMapPropagator {
	return &textMapPropagator{tracer: tracer}
}

type binaryPropagator struct {
	tracer  *tracer
	buffers sync.Pool
}

func newBinaryPropagator(tracer *tracer) *binaryPropagator {
	return &binaryPropagator{
		tracer:  tracer,
		buffers: sync.Pool{New: func() interface{} { return &bytes.Buffer{} }},
	}
}

func (p *textMapPropagator) InjectSpan(
	sp opentracing.Span,
	abstractCarrier interface{},
) error {
	sc, ok := sp.(*span)
	if !ok {
		return opentracing.ErrInvalidSpan
	}
	textMapWriter, ok := abstractCarrier.(opentracing.TextMapWriter)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}

	sc.RLock()
	defer sc.RUnlock()

	textMapWriter.Set(TracerStateHeaderName, sc.TraceContext.String())
	for k, v := range sc.baggage {
		safeKey := encodeBaggageKeyAsHeader(k)
		textMapWriter.Set(safeKey, v)
	}
	return nil
}

func (p *textMapPropagator) Join(
	operationName string,
	abstractCarrier interface{},
) (opentracing.Span, error) {
	textMapReader, ok := abstractCarrier.(opentracing.TextMapReader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	var carrier TraceContextCarrier
	err := textMapReader.ForeachKey(func(key, value string) error {
		lowerCaseKey := strings.ToLower(key)
		if lowerCaseKey == TracerStateHeaderName {
			var err error
			if carrier.TraceContext, err = ContextFromString(value); err != nil {
				return err
			}
		} else if strings.HasPrefix(lowerCaseKey, TraceBaggageHeaderPrefix) {
			if carrier.Baggage == nil {
				carrier.Baggage = make(map[string]string)
			}
			dk := decodeBaggageHeaderKey(lowerCaseKey)
			carrier.Baggage[dk] = value
		}
		return nil
	})
	if err != nil {
		p.tracer.metrics.DecodingErrors.Inc(1)
		return nil, err
	}
	if carrier.TraceContext.traceID == 0 {
		return nil, opentracing.ErrTraceNotFound
	}
	sp := p.tracer.newSpan()
	sp.TraceContext = carrier.TraceContext
	sp.baggage = carrier.Baggage
	return p.tracer.startSpanInternal(
		sp,
		operationName,
		p.tracer.timeNow(),
		nil,  // tags
		true, // join
	), nil
}

func (p *binaryPropagator) InjectSpan(
	sp opentracing.Span,
	abstractCarrier interface{},
) error {
	sc, ok := sp.(*span)
	if !ok {
		return opentracing.ErrInvalidSpan
	}
	carrier, ok := abstractCarrier.(io.Writer)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}
	var err error

	sc.RLock()
	defer sc.RUnlock()

	// Handle the tracer context
	if err := binary.Write(carrier, binary.BigEndian, sc.traceID); err != nil {
		return err
	}
	if err := binary.Write(carrier, binary.BigEndian, sc.spanID); err != nil {
		return err
	}
	if err := binary.Write(carrier, binary.BigEndian, sc.parentID); err != nil {
		return err
	}
	if err := binary.Write(carrier, binary.BigEndian, sc.flags); err != nil {
		return err
	}

	// Handle the baggage items
	if err := binary.Write(carrier, binary.BigEndian, int32(len(sc.baggage))); err != nil {
		return err
	}
	for k, v := range sc.baggage {
		if err = binary.Write(carrier, binary.BigEndian, int32(len(k))); err != nil {
			return err
		}
		io.WriteString(carrier, k)
		if err = binary.Write(carrier, binary.BigEndian, int32(len(v))); err != nil {
			return err
		}
		io.WriteString(carrier, v)
	}

	return nil
}

func (p *binaryPropagator) Join(
	operationName string,
	abstractCarrier interface{},
) (opentracing.Span, error) {
	carrier, ok := abstractCarrier.(io.Reader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	var context TraceContext

	if err := binary.Read(carrier, binary.BigEndian, &context.traceID); err != nil {
		return nil, opentracing.ErrTraceCorrupted
	}
	if err := binary.Read(carrier, binary.BigEndian, &context.spanID); err != nil {
		return nil, opentracing.ErrTraceCorrupted
	}
	if err := binary.Read(carrier, binary.BigEndian, &context.parentID); err != nil {
		return nil, opentracing.ErrTraceCorrupted
	}
	if err := binary.Read(carrier, binary.BigEndian, &context.flags); err != nil {
		return nil, opentracing.ErrTraceCorrupted
	}

	// Handle the baggage items
	var numBaggage int32
	if err := binary.Read(carrier, binary.BigEndian, &numBaggage); err != nil {
		return nil, opentracing.ErrTraceCorrupted
	}
	var baggageMap map[string]string
	if iNumBaggage := int(numBaggage); iNumBaggage > 0 {
		baggageMap = make(map[string]string, iNumBaggage)
		buf := p.buffers.Get().(*bytes.Buffer)
		defer p.buffers.Put(buf)

		var keyLen, valLen int32
		for i := 0; i < iNumBaggage; i++ {
			if err := binary.Read(carrier, binary.BigEndian, &keyLen); err != nil {
				return nil, opentracing.ErrTraceCorrupted
			}
			buf.Reset()
			buf.Grow(int(keyLen))
			if n, err := io.CopyN(buf, carrier, int64(keyLen)); err != nil || int32(n) != keyLen {
				return nil, opentracing.ErrTraceCorrupted
			}
			key := buf.String()

			if err := binary.Read(carrier, binary.BigEndian, &valLen); err != nil {
				return nil, opentracing.ErrTraceCorrupted
			}
			buf.Reset()
			buf.Grow(int(valLen))
			if n, err := io.CopyN(buf, carrier, int64(valLen)); err != nil || int32(n) != valLen {
				return nil, opentracing.ErrTraceCorrupted
			}
			baggageMap[key] = buf.String()
		}
	}

	sp := p.tracer.newSpan()
	sp.TraceContext = context
	sp.baggage = baggageMap

	return p.tracer.startSpanInternal(
		sp,
		operationName,
		p.tracer.timeNow(),
		nil,  // tags
		true, // join
	), nil
}

// Converts a baggage item key into an http header format,
// by prepending TraceBaggageHeaderPrefix and encoding the key string
func encodeBaggageKeyAsHeader(key string) string {
	// TODO encodeBaggageKeyAsHeader add caching and escaping
	return fmt.Sprintf("%v%v", TraceBaggageHeaderPrefix, key)
}

func decodeBaggageHeaderKey(key string) string {
	// TODO decodeBaggageHeaderKey add caching and escaping
	return key[len(TraceBaggageHeaderPrefix):]
}
