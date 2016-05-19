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

package client

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/uber/jaeger-client-go/crossdock/client/behavior"
	"github.com/uber/jaeger-client-go/crossdock/common"
)

// Client is a controller for the tests
type Client struct {
	ClientHostPort     string
	ServerPortHTTP     string
	ServerPortTChannel string
	listener           net.Listener
}

// Start begins a blocking Crossdock client
func (c *Client) Start() error {
	if err := c.Listen(); err != nil {
		return nil
	}
	return c.Serve()
}

// Listen initializes the server
func (c *Client) Listen() error {
	c.setDefaultPort(&c.ClientHostPort, ":"+common.DefaultClientPortHTTP)
	c.setDefaultPort(&c.ServerPortHTTP, common.DefaultServerPortHTTP)
	c.setDefaultPort(&c.ServerPortTChannel, common.DefaultServerPortTChannel)

	http.HandleFunc("/", c.behaviorRequestHandler)

	listener, err := net.Listen("tcp", c.ClientHostPort)
	if err != nil {
		return err
	}
	c.listener = listener
	c.ClientHostPort = listener.Addr().String()
	return nil
}

// Serve starts service crossdock traffic. This is a blocking call.
func (c *Client) Serve() error {
	return http.Serve(c.listener, nil)
}

// Close stops the client
func (c *Client) Close() error {
	return c.listener.Close()
}

// URL returns a URL that the client can be accessed on
func (c *Client) URL() string {
	return fmt.Sprintf("http://%s/", c.ClientHostPort)
}

func (c *Client) setDefaultPort(port *string, defaultPort string) {
	if *port == "" {
		*port = defaultPort
	}
}

func (c *Client) behaviorRequestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == "HEAD" {
		return
	}

	entries := behavior.Run(func(s behavior.Sink) {
		c.dispatch(s, httpParams{r})
	})

	enc := json.NewEncoder(w)
	if err := enc.Encode(entries); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (c *Client) dispatch(s behavior.Sink, ps behavior.Params) {
	v := ps.Param(behaviorParam)
	switch v {
	case "trace":
		c.trace(s, ps)
	default:
		behavior.Skipf(s, "unknown behavior %q", v)
	}
}

// httpParams provides access to behavior parameters that are stored inside an
// HTTP request.
type httpParams struct {
	Request *http.Request
}

func (h httpParams) Param(name string) string {
	return h.Request.FormValue(name)
}
