package client

import (
	"fmt"
	"time"

	"golang.org/x/net/context"

	"github.com/uber/jaeger-client-go/crossdock/client/behavior"
	"github.com/uber/jaeger-client-go/crossdock/common"
	"github.com/uber/jaeger-client-go/crossdock/thrift/tracetest"
	"github.com/uber/jaeger-client-go/utils"
)

func (c *Client) trace(s behavior.Sink, ps behavior.Params) {
	sampled := str2bool(ps.Param(sampledParam))
	baggage := randomBaggage()

	level1 := tracetest.NewStartTraceRequest()
	level1.Sampled = sampled
	level1.Baggage = baggage
	server1 := ps.Param(server1NameParam)

	level2 := tracetest.NewDownstream()
	level2.Host = ps.Param(server2NameParam)
	level2.Port = c.transport2port(ps.Param(server2TransportParam))
	level2.Transport = transport2transport(ps.Param(server2TransportParam))
	level2.ClientType = ps.Param(server2ClientParam)
	level1.Downstream = level2

	level3 := tracetest.NewDownstream()
	level3.Host = ps.Param(server3NameParam)
	level3.Port = c.transport2port(ps.Param(server3TransportParam))
	level3.Transport = transport2transport(ps.Param(server3TransportParam))
	level3.ClientType = ps.Param(server3ClientParam)
	level2.Downstream = level3

	url := fmt.Sprintf("http://%s:%s/start_trace", server1, c.ServerPortHTTP)
	resp, err := common.PostJSON(context.Background(), url, level1)
	if err != nil {
		behavior.Errorf(s, err.Error())
		return
	}

	traceID := resp.Span.TraceId
	if traceID == "" {
		behavior.Errorf(s, "Trace ID is empty in S1(%s)", server1)
		return
	}

	validateTrace(s, level1.Downstream, resp, server1, 1, traceID, sampled, baggage)
	if !s.HasErrors() {
		behavior.Successf(s, "trace checks out")
	}
}

func validateTrace(
	s behavior.Sink,
	target *tracetest.Downstream,
	resp *tracetest.TraceResponse,
	service string,
	level int,
	traceID string,
	sampled bool,
	baggage string) {

	if traceID != resp.Span.TraceId {
		behavior.Errorf(s, "Trace ID mismatch in S%d(%s): expected %s, received %s",
			level, service, traceID, resp.Span.TraceId)
	}
	if baggage != resp.Span.Baggage {
		behavior.Errorf(s, "Baggage mismatch in S%d(%s): expected %s, received %s",
			level, service, baggage, resp.Span.Baggage)
	}
	if sampled != resp.Span.Sampled {
		behavior.Errorf(s, "Sampled mismatch in S%d(%s): expected %s, received %s",
			level, service, sampled, resp.Span.Sampled)
	}
	if target != nil {
		if resp.Downstream == nil {
			behavior.Errorf(s, "Missing downstream in S%d(%s)", level, service)
		} else {
			validateTrace(s, target.Downstream, resp.Downstream,
				target.Host, level+1, traceID, sampled, baggage)
		}
	} else if resp.Downstream != nil {
		behavior.Errorf(s, "Unexpected downstream in S%d(%s)", level, service)
	}
}

func randomBaggage() string {
	r := utils.NewRand(time.Now().UnixNano())
	n := uint64(r.Int63())
	return fmt.Sprintf("%x", n)
}

func str2bool(v string) bool {
	switch v {
	case "true":
		return true
	case "false":
		return false
	default:
		panic(v + " is not a Boolean")
	}
}

func (c *Client) transport2port(v string) string {
	switch v {
	case "http":
		return c.ServerPortHTTP
	case "tchannel":
		return c.ServerPortTChannel
	default:
		panic("Unknown protocol " + v)
	}
}

func transport2transport(v string) tracetest.Transport {
	switch v {
	case "http":
		return tracetest.Transport_HTTP
	case "tchannel":
		return tracetest.Transport_TCHANNEL
	default:
		panic("Unknown protocol " + v)
	}
}
