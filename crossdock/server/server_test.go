package server

import (
	"fmt"
	"testing"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/crossdock/common"
	"github.com/uber/jaeger-client-go/crossdock/thrift/tracetest"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
)

func TestServerJSON(t *testing.T) {
	tracer, tCloser := jaeger.NewTracer(
		"crossdock",
		jaeger.NewConstSampler(false),
		jaeger.NewNoopReporter())
	defer tCloser.Close()

	s := &Server{HostPortHTTP: ":0", HostPortTChannel: ":0", Tracer: tracer}
	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	req := tracetest.NewStartTraceRequest()
	req.Sampled = true
	req.Baggage = "Zoidberg"
	req.Downstream = &tracetest.Downstream{
		Host:      "localhost",
		Port:      s.GetPortHTTP(),
		Transport: tracetest.Transport_HTTP,
		Downstream: &tracetest.Downstream{
			Host:      "localhost",
			Port:      s.GetPortTChannel(),
			Transport: tracetest.Transport_TCHANNEL,
		},
	}

	url := fmt.Sprintf("http://%s/start_trace", s.HostPortHTTP)
	result, err := common.PostJSON(context.Background(), url, req)

	require.NoError(t, err)
	fmt.Printf("response=%+v\n", &result)
}
