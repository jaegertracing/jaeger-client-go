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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber-go/atomic"

	"github.com/uber/jaeger-client-go/thrift-gen/baggage"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"
)

const (
	service      = "svc"
	expectedKey  = "key"
	expectedSize = int32(10)
)

var (
	testRestrictions = []*baggage.BaggageRestriction{
		{BaggageKey: expectedKey, MaxValueLength: expectedSize},
	}
)

type baggageHandler struct {
	returnError  *atomic.Bool
	restrictions []*baggage.BaggageRestriction
}

func (h *baggageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.returnError.Load() {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		bytes, _ := json.Marshal(h.restrictions)
		w.Header().Add("Content-Type", "application/json")
		w.Write(bytes)
	}
}

func (h *baggageHandler) setReturnError(b bool) {
	h.returnError.Store(b)
}

func withHTTPServer(
	restrictions []*baggage.BaggageRestriction,
	f func(
		metrics *Metrics,
		factory *metrics.LocalFactory,
		handler *baggageHandler,
		server *httptest.Server,
		tracer *Tracer,
	),
) {
	factory := metrics.NewLocalFactory(0)
	m := NewMetrics(factory, nil)

	handler := &baggageHandler{returnError: atomic.NewBool(true), restrictions: restrictions}
	server := httptest.NewServer(handler)
	defer server.Close()

	tracer, closer := NewTracer(service, NewConstSampler(true), NewNullReporter())
	defer closer.Close()

	f(m, factory, handler, server, tracer.(*Tracer))
}

func TestNewRemoteRestrictionManager(t *testing.T) {
	withHTTPServer(
		testRestrictions,
		func(
			metrics *Metrics,
			factory *metrics.LocalFactory,
			handler *baggageHandler,
			server *httptest.Server,
			tracer *Tracer,
		) {
			handler.setReturnError(false)
			mgr := newRemoteRestrictionManager(
				service,
				&BaggageRestrictionsConfig{RefreshInterval: time.Minute, ServerURL: server.URL},
				metrics,
				NullLogger,
			)
			defer mgr.Close()

			for i := 0; i < 100; i++ {
				if mgr.isReady() {
					break
				}
				time.Sleep(time.Millisecond)
			}
			require.True(t, mgr.isReady())

			span := tracer.StartSpan("s1").(*Span)
			value := "value"
			mgr.setBaggage(span, expectedKey, value)
			assert.Equal(t, value, span.BaggageItem(expectedKey))

			badKey := "bad-key"
			mgr.setBaggage(span, badKey, value)
			assert.Empty(t, span.BaggageItem(badKey))

			testutils.AssertCounterMetrics(t, factory,
				testutils.ExpectedMetric{
					Name:  "jaeger.baggage-restrictions-update",
					Tags:  map[string]string{"result": "ok"},
					Value: 1,
				},
			)
		})
}

func TestFailClosed(t *testing.T) {
	withHTTPServer(
		testRestrictions,
		func(
			metrics *Metrics,
			factory *metrics.LocalFactory,
			handler *baggageHandler,
			server *httptest.Server,
			tracer *Tracer,
		) {
			mgr := newRemoteRestrictionManager(
				service,
				&BaggageRestrictionsConfig{
					RefreshInterval:                    time.Minute,
					ServerURL:                          server.URL,
					DenyBaggageOnInitializationFailure: true,
				},
				metrics,
				NullLogger,
			)
			require.False(t, mgr.isReady())

			testutils.AssertCounterMetrics(t, factory,
				testutils.ExpectedMetric{
					Name:  "jaeger.baggage-restrictions-update",
					Tags:  map[string]string{"result": "err"},
					Value: 1,
				},
			)

			// FailClosed should not allow any key to be written
			span := tracer.StartSpan("s1").(*Span)
			value := "value"
			mgr.setBaggage(span, expectedKey, value)
			assert.Empty(t, span.BaggageItem(expectedKey))

			handler.setReturnError(false)
			mgr.updateRestrictions()

			// Wait until manager retrieves baggage restrictions
			for i := 0; i < 100; i++ {
				if mgr.isReady() {
					break
				}
				time.Sleep(time.Millisecond)
			}
			require.True(t, mgr.isReady())

			mgr.setBaggage(span, expectedKey, value)
			assert.Equal(t, value, span.BaggageItem(expectedKey))
		})
}

func TestFailOpen(t *testing.T) {
	withHTTPServer(
		testRestrictions,
		func(
			metrics *Metrics,
			factory *metrics.LocalFactory,
			handler *baggageHandler,
			server *httptest.Server,
			tracer *Tracer,
		) {
			mgr := newRemoteRestrictionManager(
				service,
				&BaggageRestrictionsConfig{RefreshInterval: time.Millisecond, ServerURL: server.URL},
				metrics,
				NullLogger,
			)
			require.False(t, mgr.isReady())

			// FailOpn should allow any key to be written
			span := tracer.StartSpan("s1").(*Span)
			value := "value"
			mgr.setBaggage(span, expectedKey, value)
			assert.Equal(t, value, span.BaggageItem(expectedKey))
		})
}
