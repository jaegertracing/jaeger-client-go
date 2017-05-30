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

package jaeger

import (
	"time"
)

const (
	defaultRefreshInterval                    = time.Minute
	defaultBaggageRestrictionManagerServerURL = "http://localhost:5778/baggage"
)

// BaggageRestrictionManagerOption is a function that sets some option on the BaggageRestrictionManager
type BaggageRestrictionManagerOption func(options *baggageRestrictionManagerOptions)

// BaggageRestrictionManagerOptions is a factory for all available BaggageRestrictionManagerOption's
var BaggageRestrictionManagerOptions baggageRestrictionManagerOptions

type baggageRestrictionManagerOptions struct {
	metrics                            *Metrics
	logger                             Logger
	baggageRestrictionManagerServerURL string
	refreshInterval                    time.Duration
}

// Metrics creates a BaggageRestrictionManagerOption that initializes Metrics on the BaggageRestrictionManager,
// which is used to emit statistics.
func (baggageRestrictionManagerOptions) Metrics(m *Metrics) BaggageRestrictionManagerOption {
	return func(o *baggageRestrictionManagerOptions) {
		o.metrics = m
	}
}

// Logger creates a BaggageRestrictionManagerOption that sets the logger used by the BaggageRestrictionManager.
func (baggageRestrictionManagerOptions) Logger(logger Logger) BaggageRestrictionManagerOption {
	return func(o *baggageRestrictionManagerOptions) {
		o.logger = logger
	}
}

// BaggageRestrictionManagerServerURL creates a BaggageRestrictionManagerOption that sets the baggage restriction manager
// server url of the local agent that contains the baggage restrictions.
func (baggageRestrictionManagerOptions) BaggageRestrictionManagerServerURL(url string) BaggageRestrictionManagerOption {
	return func(o *baggageRestrictionManagerOptions) {
		o.baggageRestrictionManagerServerURL = url
	}
}

// RefreshInterval creates a BaggageRestrictionManagerOption that sets how often the
// BaggageRestrictionManager will poll local agent for the appropriate baggage restrictions.
func (baggageRestrictionManagerOptions) RefreshInterval(refreshInterval time.Duration) BaggageRestrictionManagerOption {
	return func(o *baggageRestrictionManagerOptions) {
		o.refreshInterval = refreshInterval
	}
}

func applyOptions(options ...BaggageRestrictionManagerOption) baggageRestrictionManagerOptions {
	opts := baggageRestrictionManagerOptions{}
	for _, option := range options {
		option(&opts)
	}
	if opts.metrics == nil {
		opts.metrics = NewNullMetrics()
	}
	if opts.logger == nil {
		opts.logger = NullLogger
	}
	if opts.baggageRestrictionManagerServerURL == "" {
		opts.baggageRestrictionManagerServerURL = defaultBaggageRestrictionManagerServerURL
	}
	if opts.refreshInterval == 0 {
		opts.refreshInterval = defaultRefreshInterval
	}
	return opts
}
