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

package baggage

import (
	"time"

	"github.com/uber/jaeger-client-go"
)

const (
	defaultRefreshInterval = time.Minute
	defaultServerURL       = "http://localhost:5778/baggageRestrictions"
)

// Option is a function that sets some option on the RemoteRestrictionManager
type Option func(options *options)

// Options is a factory for all available options
var Options options

type options struct {
	metrics         *jaeger.Metrics
	logger          jaeger.Logger
	serverURL       string
	refreshInterval time.Duration
}

// Metrics creates an Option that initializes Metrics on the RemoteRestrictionManager, which is used to emit statistics.
func (options) Metrics(m *jaeger.Metrics) Option {
	return func(o *options) {
		o.metrics = m
	}
}

// Logger creates an Option that sets the logger used by the RemoteRestrictionManager.
func (options) Logger(logger jaeger.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// ServerURL creates an Option that sets the server url of the local agent that contains the baggage restrictions.
func (options) ServerURL(url string) Option {
	return func(o *options) {
		o.serverURL = url
	}
}

// RefreshInterval creates an Option that sets how often the RemoteRestrictionManager will poll local agent for
// the baggage restrictions.
func (options) RefreshInterval(refreshInterval time.Duration) Option {
	return func(o *options) {
		o.refreshInterval = refreshInterval
	}
}

func applyOptions(o ...Option) options {
	opts := options{}
	for _, option := range o {
		option(&opts)
	}
	if opts.metrics == nil {
		opts.metrics = jaeger.NewNullMetrics()
	}
	if opts.logger == nil {
		opts.logger = jaeger.NullLogger
	}
	if opts.serverURL == "" {
		opts.serverURL = defaultServerURL
	}
	if opts.refreshInterval == 0 {
		opts.refreshInterval = defaultRefreshInterval
	}
	return opts
}
