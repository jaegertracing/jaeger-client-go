// Copyright (c) 2016 Uber Technologies, Inc.
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

// Injector is responsible for injecting SpanContext instances in a manner suitable
// for propagation via a format-specific "carrier" object. Typically the
// injection will take place across an RPC boundary, but message queues and
// other IPC mechanisms are also reasonable places to use an Injector.
type Injector interface {
	// Inject takes `SpanContext` and injects it into `carrier`. The actual type
	// of `carrier` depends on the `format` passed to `Tracer.Inject()`.
	//
	// Implementations may return opentracing.ErrInvalidCarrier or any other
	// implementation-specific error if injection fails.
	Inject(ctx *SpanContext, carrier interface{}) error
}

// Extractor is responsible for extracting SpanContext instances from a
// format-specific "carrier" object. Typically the extraction will take place
// on the server side of an RPC boundary, but message queues and other IPC
// mechanisms are also reasonable places to use an Extractor.
type Extractor interface {
	// Extract decodes a SpanContext instance from the given `carrier`,
	// or (nil, opentracing.ErrSpanContextNotFound) if no context could
	// be found in the `carrier`.
	Extract(carrier interface{}) (*SpanContext, error)
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

func (p *textMapPropagator) Inject(
	sc *SpanContext,
	abstractCarrier interface{},
) error {
	textMapWriter, ok := abstractCarrier.(opentracing.TextMapWriter)
	if !ok {
		return opentracing.ErrInvalidCarrier
	}

	sc.RLock()
	defer sc.RUnlock()

	textMapWriter.Set(TracerStateHeaderName, sc.String())
	for k, v := range sc.baggage {
		safeKey := encodeBaggageKeyAsHeader(k)
		textMapWriter.Set(safeKey, v)
	}
	return nil
}

func (p *textMapPropagator) Extract(abstractCarrier interface{}) (*SpanContext, error) {
	textMapReader, ok := abstractCarrier.(opentracing.TextMapReader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	var ctx *SpanContext
	var baggage map[string]string
	err := textMapReader.ForeachKey(func(key, value string) error {
		lowerCaseKey := strings.ToLower(key)
		if lowerCaseKey == TracerStateHeaderName {
			var err error
			if ctx, err = ContextFromString(value); err != nil {
				return err
			}
		} else if strings.HasPrefix(lowerCaseKey, TraceBaggageHeaderPrefix) {
			if baggage == nil {
				baggage = make(map[string]string)
			}
			dk := decodeBaggageHeaderKey(lowerCaseKey)
			baggage[dk] = value
		}
		return nil
	})
	if err != nil {
		p.tracer.metrics.DecodingErrors.Inc(1)
		return nil, err
	}
	if ctx == nil || ctx.traceID == 0 {
		return nil, opentracing.ErrSpanContextNotFound
	}
	ctx.baggage = baggage
	return ctx, nil
}

func (p *binaryPropagator) Inject(
	sc *SpanContext,
	abstractCarrier interface{},
) error {
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

func (p *binaryPropagator) Extract(abstractCarrier interface{}) (*SpanContext, error) {
	carrier, ok := abstractCarrier.(io.Reader)
	if !ok {
		return nil, opentracing.ErrInvalidCarrier
	}
	ctx := new(SpanContext)

	if err := binary.Read(carrier, binary.BigEndian, &ctx.traceID); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	if err := binary.Read(carrier, binary.BigEndian, &ctx.spanID); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	if err := binary.Read(carrier, binary.BigEndian, &ctx.parentID); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	if err := binary.Read(carrier, binary.BigEndian, &ctx.flags); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}

	// Handle the baggage items
	var numBaggage int32
	if err := binary.Read(carrier, binary.BigEndian, &numBaggage); err != nil {
		return nil, opentracing.ErrSpanContextCorrupted
	}
	var baggage map[string]string
	if iNumBaggage := int(numBaggage); iNumBaggage > 0 {
		baggage = make(map[string]string, iNumBaggage)
		buf := p.buffers.Get().(*bytes.Buffer)
		defer p.buffers.Put(buf)

		var keyLen, valLen int32
		for i := 0; i < iNumBaggage; i++ {
			if err := binary.Read(carrier, binary.BigEndian, &keyLen); err != nil {
				return nil, opentracing.ErrSpanContextCorrupted
			}
			buf.Reset()
			buf.Grow(int(keyLen))
			if n, err := io.CopyN(buf, carrier, int64(keyLen)); err != nil || int32(n) != keyLen {
				return nil, opentracing.ErrSpanContextCorrupted
			}
			key := buf.String()

			if err := binary.Read(carrier, binary.BigEndian, &valLen); err != nil {
				return nil, opentracing.ErrSpanContextCorrupted
			}
			buf.Reset()
			buf.Grow(int(valLen))
			if n, err := io.CopyN(buf, carrier, int64(valLen)); err != nil || int32(n) != valLen {
				return nil, opentracing.ErrSpanContextCorrupted
			}
			baggage[key] = buf.String()
		}
	}

	ctx.baggage = baggage
	return ctx, nil
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
