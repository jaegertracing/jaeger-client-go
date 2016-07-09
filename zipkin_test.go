package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/opentracing/opentracing-go/ext"
)

func TestZipkinPropagator(t *testing.T) {
	tracer, tCloser := NewTracer("x", NewConstSampler(true), NewNullReporter())
	defer tCloser.Close()

	carrier := &TestZipkinSpan{}
	sp := tracer.StartSpan("y")

	// Note: we intentionally use string as format, as that's what TChannel would need to do
	if err := tracer.Inject(sp.Context(), "zipkin-span-format", carrier); err != nil {
		t.Fatalf("Inject failed: %+v", err)
	}
	sp1 := sp.(*span)
	assert.Equal(t, sp1.context.traceID, carrier.traceID)
	assert.Equal(t, sp1.context.spanID, carrier.spanID)
	assert.Equal(t, sp1.context.parentID, carrier.parentID)
	assert.Equal(t, sp1.context.flags, carrier.flags)

	sp2ctx, err := tracer.Extract("zipkin-span-format", carrier)
	if err != nil {
		t.Fatalf("Extract failed: %+v", err)
	}
	sp2 := tracer.StartSpan("x", ext.RPCServerOption(sp2ctx))
	sp3 := sp2.(*span)
	assert.Equal(t, sp1.context.traceID, sp3.context.traceID)
	assert.Equal(t, sp1.context.spanID, sp3.context.spanID)
	assert.Equal(t, sp1.context.parentID, sp3.context.parentID)
	assert.Equal(t, sp1.context.flags, sp3.context.flags)
}

// TestZipkinSpan is a mock-up of TChannel's internal Span struct
type TestZipkinSpan struct {
	traceID  uint64
	parentID uint64
	spanID   uint64
	flags    byte
}

func (s TestZipkinSpan) TraceID() uint64              { return s.traceID }
func (s TestZipkinSpan) ParentID() uint64             { return s.parentID }
func (s TestZipkinSpan) SpanID() uint64               { return s.spanID }
func (s TestZipkinSpan) Flags() byte                  { return s.flags }
func (s *TestZipkinSpan) SetTraceID(traceID uint64)   { s.traceID = traceID }
func (s *TestZipkinSpan) SetSpanID(spanID uint64)     { s.spanID = spanID }
func (s *TestZipkinSpan) SetParentID(parentID uint64) { s.parentID = parentID }
func (s *TestZipkinSpan) SetFlags(flags byte)         { s.flags = flags }
