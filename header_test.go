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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetDefaultOrCustom(t *testing.T) {
	assert.Equal(t, (&HeadersConfig{}).applyDefaults(), getDefaultHeadersConfig())
	assert.Equal(t, (&HeadersConfig{
		JaegerDebugHeader: "custom-jaeger-debug-header",
	}).applyDefaults(), &HeadersConfig{
		JaegerDebugHeader:        "custom-jaeger-debug-header",
		JaegerBaggageHeader:      JaegerBaggageHeader,
		TraceContextHeaderName:   TraceContextHeaderName,
		TraceBaggageHeaderPrefix: TraceBaggageHeaderPrefix,
	})

	customHeaders := &HeadersConfig{
		JaegerDebugHeader:        "custom-jaeger-debug-header",
		JaegerBaggageHeader:      "custom-jaeger-baggage-header",
		TraceContextHeaderName:   "custom-tracer-state-header-name",
		TraceBaggageHeaderPrefix: "custom-tracer-baggage-header-prefix",
	}
	assert.Equal(t, customHeaders.applyDefaults(), customHeaders)
}

func TestGetDefaultHeadersConfig(t *testing.T) {
	assert.Equal(t, getDefaultHeadersConfig(), &HeadersConfig{
		JaegerDebugHeader:        JaegerDebugHeader,
		JaegerBaggageHeader:      JaegerBaggageHeader,
		TraceContextHeaderName:   TraceContextHeaderName,
		TraceBaggageHeaderPrefix: TraceBaggageHeaderPrefix,
	})
}
