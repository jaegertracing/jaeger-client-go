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
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"github.com/opentracing/opentracing-go"
)

const (
	flagSampled = byte(1)
	flagDebug   = byte(2)
)

var (
	errEmptyTracerStateString     = errors.New("Cannot convert empty string to tracer state")
	errMalformedTracerStateString = errors.New("String does not match tracer state format")
)

// TraceContext represents propagated span identity and state
type SpanContext struct {
	sync.RWMutex

	// traceID represents globally unique ID of the trace.
	// Usually generated as a random number.
	traceID uint64

	// spanID represents span ID that must be unique within its trace,
	// but does not have to be globally unique.
	spanID uint64

	// parentID refers to the ID of the parent span.
	// Should be 0 if the current span is a root span.
	parentID uint64

	// flags is a bitmap containing such bits as 'sampled' and 'debug'.
	flags byte

	// Distributed Context baggage
	baggage map[string]string
}

func (s *SpanContext) SetBaggageItem(key, value string) opentracing.SpanContext {
	key = normalizeBaggageKey(key)
	s.Lock()
	defer s.Unlock()
	if s.baggage == nil {
		s.baggage = make(map[string]string)
	}
	s.baggage[key] = value
	return s
}

func (c *SpanContext) BaggageItem(key string) string {
	key = normalizeBaggageKey(key)
	c.RLock()
	defer c.RUnlock()
	return c.baggage[key]
}

func (c *SpanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	c.RLock()
	defer c.RUnlock()
	for k, v := range c.baggage {
		if !handler(k, v) {
			break
		}
	}
}

// IsSampled returns whether this trace was chosen for permanent storage
// by the sampling mechanism of the tracer.
func (c *SpanContext) IsSampled() bool {
	return (c.flags & flagSampled) == flagSampled
}

func (c *SpanContext) String() string {
	return fmt.Sprintf("%x:%x:%x:%x", c.traceID, c.spanID, c.parentID, c.flags)
}

// ContextFromString reconstructs the Context encoded in a string
func ContextFromString(value string) (*SpanContext, error) {
	var context = new(SpanContext)
	if value == "" {
		return nil, errEmptyTracerStateString
	}
	parts := strings.Split(value, ":")
	if len(parts) != 4 {
		return nil, errMalformedTracerStateString
	}
	var err error
	if context.traceID, err = strconv.ParseUint(parts[0], 16, 64); err != nil {
		return nil, err
	}
	if context.spanID, err = strconv.ParseUint(parts[1], 16, 64); err != nil {
		return nil, err
	}
	if context.parentID, err = strconv.ParseUint(parts[2], 16, 64); err != nil {
		return nil, err
	}
	flags, err := strconv.ParseUint(parts[3], 10, 8)
	if err != nil {
		return nil, err
	}
	context.flags = byte(flags)
	return context, nil
}

// TraceID implements TraceID() of SpanID
func (c SpanContext) TraceID() uint64 {
	return c.traceID
}

// SpanID implements SpanID() of SpanID
func (c SpanContext) SpanID() uint64 {
	return c.spanID
}

// ParentID implements ParentID() of SpanID
func (c SpanContext) ParentID() uint64 {
	return c.parentID
}

// NewTraceContext creates a new instance of TraceContext
func NewTraceContext(traceID, spanID, parentID uint64, sampled bool) *SpanContext {
	flags := byte(0)
	if sampled {
		flags = flagSampled
	}
	return &SpanContext{
		traceID:  traceID,
		spanID:   spanID,
		parentID: parentID,
		flags:    flags}
}

func (c *SpanContext) CopyFrom(ctx *SpanContext) {
	c.Lock()
	defer c.Unlock()

	ctx.RLock()
	defer ctx.RUnlock()

	c.traceID = ctx.traceID
	c.spanID = ctx.spanID
	c.parentID = ctx.parentID
	c.flags = ctx.flags
	if l := len(ctx.baggage); l > 0 {
		c.baggage = make(map[string]string, l)
		for k, v := range ctx.baggage {
			c.baggage[k] = v
		}
	} else {
		c.baggage = nil
	}
}