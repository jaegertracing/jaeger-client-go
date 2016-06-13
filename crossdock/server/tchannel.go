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

package server

import (
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
	"golang.org/x/net/context"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/crossdock/thrift/tracetest"
)

func (s *Server) startTChannelServer() error {
	var traceSampleRate float64
	channelOpts := &tchannel.ChannelOptions{
		TraceSampleRate: &traceSampleRate,
	}
	ch, err := tchannel.NewChannel("go", channelOpts)
	if err != nil {
		return err
	}
	server := thrift.NewServer(ch)

	s.channel = ch

	handler := tracetest.NewTChanTracedServiceServer(s)
	server.Register(handler)

	if err := ch.ListenAndServe(s.HostPortTChannel); err != nil {
		return err
	}
	s.HostPortTChannel = ch.PeerInfo().HostPort
	fmt.Printf("Started tchannel server at %s\n", s.HostPortTChannel)
	return nil
}

// StartTrace implements StartTrace() of TChanTracedService
func (s *Server) StartTrace(ctx thrift.Context, request *tracetest.StartTraceRequest) (*tracetest.TraceResponse, error) {
	return nil, errCannotStartInTChannel
}

// JoinTrace implements JoinTrace() of TChanTracedService
func (s *Server) JoinTrace(ctx thrift.Context, request *tracetest.JoinTraceRequest) (*tracetest.TraceResponse, error) {
	println("tchannel server handling JoinTrace")
	ctx2 := setupOpenTracingContext(s.Tracer, ctx, "tchannel", ctx.Headers())
	return s.prepareResponse(ctx2, request.ServerRole, request.Downstream)
}

func (s *Server) callDownstreamTChannel(ctx context.Context, target *tracetest.Downstream) (*tracetest.TraceResponse, error) {
	req := &tracetest.JoinTraceRequest{
		ServerRole: target.ServerRole,
		Downstream: target.Downstream,
	}

	hostPort := fmt.Sprintf("%s:%s", target.Host, target.Port)
	fmt.Printf("calling downstream '%s' over tchannel:%s\n", target.ServiceName, hostPort)

	ch, err := tchannel.NewChannel("tchannel-client", nil)
	if err != nil {
		return nil, err
	}

	opts := &thrift.ClientOptions{HostPort: hostPort}
	thriftClient := thrift.NewClient(ch, target.ServiceName, opts)

	client := tracetest.NewTChanTracedServiceClient(thriftClient)

	// Manual bridging of OpenTracing Span into TChannel Span
	tCtx, cx := WrapContext(ctx, time.Second)
	defer cx()

	return client.JoinTrace(tCtx, req)
}

// WrapContext takes a regular `net.Context` and converts it into `thrift.Context` suitable
// for outbound calls to TChannel servers.  If the input context contains a OpoenTracing span,
// that span is converted to TChannel-compatible Span.
func WrapContext(ctx context.Context, timeout time.Duration) (thrift.Context, context.CancelFunc) {
	builder := tchannel.NewContextBuilder(timeout)
	convertOpenTracingSpan(ctx, builder)
	return builder.Build()
}

// convertOpenTracingSpan extracts an OpenTracing Span from the context and converts it
// into TChannel Span representation, so that it is used by the outbound TChannel call
// to create a child span.
//
// Once TChannel supports OpenTracing API directly, this bridging will not be required.
func convertOpenTracingSpan(ctx context.Context, builder *tchannel.ContextBuilder) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return
	}
	carrier := &jaeger.TraceContextCarrier{}
	if err := span.Tracer().Inject(span, jaeger.TraceContextFormat, carrier); err != nil {
		return
	}
	sc := carrier.TraceContext
	builder.SetExternalSpan(sc.TraceID(), sc.SpanID(), sc.ParentID(), sc.IsSampled())
	for k, v := range carrier.Baggage {
		builder.AddHeader(k, v)
	}
}

// setupOpenTracingContext extracts a TChannel tracing Span from the context, converts
// it into OpenTracing Span, and returns a thrift.Context containing both spans.
// This allows the returned thrift.Context to be used for both TChannel and HTTP
// outbound calls and still maintain uninterrupted trace.
//
// If `tracer` is nil, `opentracing.GlobalTracer()` is used.
//
// Note that we never call finish() on the OpenTracing span, because it is actually a double
// of the real inbound TChannel Span, which will be reported to Jaeger. The new OpenTracing
// span is used merely to allow creating children spans from it.
//
// Once TChannel supports OpenTracing API directly, this bridging will not be required.
//
func setupOpenTracingContext(tracer opentracing.Tracer, ctx context.Context, method string, headers map[string]string) thrift.Context {
	tSpan := tchannel.CurrentSpan(ctx)
	if tSpan != nil {
		// populate a fake carrier and try to create OpenTracing Span
		carrier := &jaeger.TraceContextCarrier{Baggage: headers}
		carrier.TraceContext = *jaeger.NewTraceContext(
			tSpan.TraceID(), tSpan.SpanID(), tSpan.ParentID(), tSpan.TracingEnabled())
		if tracer == nil {
			tracer = opentracing.GlobalTracer()
		}
		if span, err := tracer.Join(method, jaeger.TraceContextFormat, carrier); err == nil {
			ctx = opentracing.ContextWithSpan(ctx, span)
		}
	}
	return thrift.WithHeaders(ctx, headers)
}
