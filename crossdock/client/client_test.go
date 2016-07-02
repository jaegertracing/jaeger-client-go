package client

import (
	"fmt"
	"log"
	"net/url"
	"testing"

	"github.com/crossdock/crossdock-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/crossdock/common"
	"github.com/uber/jaeger-client-go/crossdock/server"
)

func TestCrossdock(t *testing.T) {
	log.Println("Starting crossdock test")

	tracer, tCloser := jaeger.NewTracer(
		"crossdock-go",
		jaeger.NewConstSampler(false),
		jaeger.NewInMemoryReporter())
	defer tCloser.Close()

	s := &server.Server{
		HostPortHTTP:     "127.0.0.1:0",
		HostPortTChannel: "127.0.0.1:0",
		Tracer:           tracer,
	}
	err := s.Start()
	require.NoError(t, err)
	defer s.Close()

	c := &Client{
		ClientHostPort:     "127.0.0.1:0",
		ServerPortHTTP:     s.GetPortHTTP(),
		ServerPortTChannel: s.GetPortTChannel(),
		hostMapper:         func(server string) string { return "localhost" },
	}
	err = c.AsyncStart()
	require.NoError(t, err)
	defer c.Close()

	serverURL := fmt.Sprintf("http://%s/", s.HostPortHTTP)

	crossdock.Wait(t, serverURL, 10)
	crossdock.Wait(t, c.ClientURL, 10)

	type params map[string]string
	type axes map[string][]string

	behaviors := []struct {
		name string
		axes axes
	}{
		{
			name: behaviorTrace,
			axes: axes{
				server1NameParam:      []string{common.DefaultServiceName},
				sampledParam:          []string{"true", "false"},
				server2NameParam:      []string{common.DefaultServiceName},
				server2TransportParam: []string{transportHTTP, transportTChannel},
				server3NameParam:      []string{common.DefaultServiceName},
				server3TransportParam: []string{transportHTTP, transportTChannel},
			},
		},
	}

	for _, bb := range behaviors {
		for _, entry := range crossdock.Combinations(bb.axes) {
			entryArgs := url.Values{}
			for k, v := range entry {
				entryArgs.Set(k, v)
			}
			// test via real HTTP call
			crossdock.Call(t, c.ClientURL, bb.name, entryArgs)
		}
	}
}

func TestHostMapper(t *testing.T) {
	c := &Client{
		ClientHostPort:     "127.0.0.1:0",
		ServerPortHTTP:     "8080",
		ServerPortTChannel: "8081",
	}
	assert.Equal(t, "go", c.mapServiceToHost("go"))
	c.hostMapper = func(server string) string { return "localhost" }
	assert.Equal(t, "localhost", c.mapServiceToHost("go"))
}
