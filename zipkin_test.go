package jaeger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestZipkinPropagator(t *testing.T) {
	tracer, tCloser := NewTracer("x", NewConstSampler(true), NewNullReporter())
	defer tCloser.Close()

	carrier := &TestZipkinSpan{}
	sp := tracer.StartSpan("y")

	// Note: we intentionally use string as format, as that's what TChannel would need to do
	if err := tracer.Inject(sp, "zipkin-span-format", carrier); err != nil {
		t.Fatalf("Inject failed: %+v", err)
	}
	sp1 := sp.(*span)
	assert.Equal(t, sp1.traceID, carrier.traceID)
	assert.Equal(t, sp1.spanID, carrier.spanID)
	assert.Equal(t, sp1.parentID, carrier.parentID)
	assert.Equal(t, sp1.flags, carrier.flags)

	sp2, err := tracer.Join("z", "zipkin-span-format", carrier)
	if err != nil {
		t.Fatalf("Extract failed: %+v", err)
	}
	sp3 := sp2.(*span)
	assert.Equal(t, sp1.traceID, sp3.traceID)
	assert.Equal(t, sp1.spanID, sp3.spanID)
	assert.Equal(t, sp1.parentID, sp3.parentID)
	assert.Equal(t, sp1.flags, sp3.flags)
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
