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

import "time"

const (
	defaultRefreshInterval = time.Minute
	defaultServerURL       = "http://localhost:5778/baggageRestrictions"
)

// BaggageRestrictionsConfig configures the baggage restrictions manager which can be used to white-list
// certain baggage keys. All fields are optional.
type BaggageRestrictionsConfig struct {
	// DenyBaggageOnInitializationFailure controls the startup failure mode of the baggage restriction
	// manager. If true, the manager will not allow any baggage to be written until baggage restrictions have
	// been retrieved from jaeger-agent. If false, the manager wil allow any baggage to be written until baggage
	// restrictions have been retrieved from jaeger-agent.
	DenyBaggageOnInitializationFailure bool `yaml:"denyBaggageOnInitializationFailure"`

	// ServerURL is the address of jaeger-agent's HTTP baggage restrictions server
	ServerURL string `yaml:"serverURL"`

	// RefreshInterval controls how often the baggage restriction manager will poll
	// jaeger-agent for the most recent baggage restrictions.
	RefreshInterval time.Duration `yaml:"refreshInterval"`
}

func (c *BaggageRestrictionsConfig) applyDefaults() *BaggageRestrictionsConfig {
	if c.RefreshInterval == 0 {
		c.RefreshInterval = defaultRefreshInterval
	}
	if c.ServerURL == "" {
		c.ServerURL = defaultServerURL
	}
	return c
}
