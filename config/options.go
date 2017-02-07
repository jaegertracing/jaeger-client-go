// Copyright (c) 2017 Uber Technologies, Inc.
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

package config

import (
	"github.com/uber/jaeger-lib/metrics"

	"github.com/uber/jaeger-client-go"
)

// ClientOption is a function that sets some option on the client
type ClientOption func(c *Configuration)

// ClientOptions is a factory for all available ClientOption's
var ClientOptions clientOptions

type clientOptions struct{}

// WithMetrics creates a ClientOption that initializes Metrics in the client,
// which is used to emit statistics.
func (clientOptions) WithMetrics(factory metrics.Factory) func(*Configuration) {
	return func(c *Configuration) {
		c.clientMetrics = jaeger.NewMetrics(factory, nil)
	}
}

// WithLogger can be provided to log Reporter errors, as well as to log spans
// if Reporter.LogSpans is set to true.
func (clientOptions) WithLogger(logger jaeger.Logger) func(*Configuration) {
	return func(c *Configuration) {
		c.logger = logger
	}
}
