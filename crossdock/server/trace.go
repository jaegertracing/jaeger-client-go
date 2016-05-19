package server

import (
	"fmt"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/crossdock/common"
	"github.com/uber/jaeger-client-go/crossdock/thrift/tracetest"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"golang.org/x/net/context"
)

func (s *Server) doStartTrace(req *tracetest.StartTraceRequest) (*tracetest.TraceResponse, error) {
	span := s.Tracer.StartSpan("s1")
	if req.Sampled {
		ext.SamplingPriority.Set(span, 1)
	}
	span.SetBaggageItem(BaggageKey, req.Baggage)
	defer span.Finish()

	ctx := opentracing.ContextWithSpan(context.Background(), span)

	return s.prepareResponse(ctx, req.Downstream)
}

func (s *Server) doJoinTrace(ctx context.Context, req *tracetest.JoinTraceRequest) (*tracetest.TraceResponse, error) {
	return s.prepareResponse(ctx, req.Downstream)
}

func (s *Server) prepareResponse(ctx context.Context, reqDwn *tracetest.Downstream) (*tracetest.TraceResponse, error) {
	observedSpan, err := observeSpan(ctx, s.Tracer)
	if err != nil {
		return nil, err
	}

	resp := tracetest.NewTraceResponse()
	resp.Span = observedSpan

	if reqDwn != nil {
		downstreamResp, err := s.callDownstream(ctx, reqDwn)
		if err != nil {
			return nil, err
		}
		resp.Downstream = downstreamResp
	}

	return resp, nil
}

func (s *Server) callDownstream(ctx context.Context, downstream *tracetest.Downstream) (*tracetest.TraceResponse, error) {
	switch downstream.Transport {
	case tracetest.Transport_HTTP:
		return s.callDownstreamHTTP(ctx, downstream)
	case tracetest.Transport_TCHANNEL:
		return s.callDownstreamTChannel(ctx, downstream)
	default:
		return nil, errUnrecognizedProtocol
	}
}

func (s *Server) callDownstreamHTTP(ctx context.Context, downstream *tracetest.Downstream) (*tracetest.TraceResponse, error) {
	req := &tracetest.JoinTraceRequest{Downstream: downstream.Downstream}
	url := fmt.Sprintf("http://%s:%s/join_trace", downstream.Host, downstream.Port)
	println("Calling downstream at", url)
	return common.PostJSON(ctx, url, req)
}

func observeSpan(ctx context.Context, tracer opentracing.Tracer) (*tracetest.ObservedSpan, error) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return nil, errNoSpanObserved
	}
	c := jaeger.TraceContextCarrier{}
	if err := tracer.Inject(span, jaeger.TraceContextFormat, &c); err != nil {
		return nil, err
	}
	observedSpan := tracetest.NewObservedSpan()
	observedSpan.TraceId = fmt.Sprintf("%x", c.TraceContext.TraceID())
	observedSpan.Sampled = c.TraceContext.IsSampled()
	observedSpan.Baggage = span.BaggageItem(BaggageKey)
	return observedSpan, nil
}
