// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/internal/throttler"
	"github.com/uber/jaeger-client-go/log"
)

var _ throttler.Throttler = &Throttler{}
var _ io.Closer = &Throttler{}
var _ jaeger.ProcessSetter = &Throttler{}

var testOperation = "op"

type creditHandler struct {
	returnError bool
	credits     float64
}

func (h *creditHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.returnError {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		operations := r.URL.Query()["operation"]
		resp := make([]creditResponse, len(operations))
		for i, op := range operations {
			resp[i] = creditResponse{Operation: op, Credits: h.credits}
		}
		h.credits = 0
		bytes, _ := json.Marshal(resp)
		w.Header().Add("Content-Type", "application/json")
		w.Write(bytes)
	}
}

func (h *creditHandler) setReturnError(b bool) {
	h.returnError = b
}

func withHTTPServer(
	credits float64,
	f func(
		handler *creditHandler,
		server *httptest.Server,
	),
) {
	handler := &creditHandler{returnError: false, credits: credits}
	server := httptest.NewServer(handler)
	defer server.Close()

	f(handler, server)
}

func TestCreditManager(t *testing.T) {
	withHTTPServer(
		2,
		func(
			handler *creditHandler,
			server *httptest.Server,
		) {
			logger := &log.BytesBufferLogger{}
			creditManager := newHTTPCreditManagerProxy(getHostPort(t, server.URL), logger)
			credits := creditManager.FetchCredits("uuid", "svc", []string{"op1", "op2"})
			require.Len(t, credits, 2)
			assert.EqualValues(t, 2, credits[0].Credits)

			credits = creditManager.FetchCredits("uuid", "svc", []string{"op1"})
			require.Len(t, credits, 1)
			assert.EqualValues(t, 0, credits[0].Credits)

			handler.setReturnError(true)
			credits = creditManager.FetchCredits("uuid", "svc", []string{"op1"})
			assert.Empty(t, credits)
			assert.Equal(t, "ERROR: Failed to receive credits from agent: StatusCode: 500, Body: \n", logger.String())
		})
}

func TestRemoteThrottler_UUIDNotSet(t *testing.T) {
	withHTTPServer(
		2,
		func(
			handler *creditHandler,
			server *httptest.Server,
		) {
			logger := &log.BytesBufferLogger{}
			creditManager := newHTTPCreditManagerProxy(getHostPort(t, server.URL), jaeger.NullLogger)
			throttler := &Throttler{
				creditManager: creditManager,
				service:       "svc",
				credits:       make(map[string]float64),
				options:       options{logger: logger, synchronousInitialization: true},
			}
			assert.False(t, throttler.IsAllowed(testOperation))
			assert.Equal(t, "ERROR: Throttler uuid is not set, failed to fetch credits\n", logger.String())
			logger.Flush()
			throttler.SetProcess(jaeger.Process{UUID: ""})
			assert.False(t, throttler.IsAllowed(testOperation))
			assert.Equal(t, "ERROR: Throttler uuid is not set, failed to fetch credits\n", logger.String())
			throttler.SetProcess(jaeger.Process{UUID: "uuid"})
			assert.True(t, throttler.IsAllowed(testOperation))
			assert.True(t, throttler.IsAllowed(testOperation))
			assert.False(t, throttler.IsAllowed(testOperation))
		})
}

func TestRemotelyControlledThrottler_pollManager(t *testing.T) {
	withHTTPServer(
		2,
		func(
			handler *creditHandler,
			server *httptest.Server,
		) {
			throttler := NewThrottler(
				"svc",
				Options.RefreshInterval(time.Millisecond),
				Options.HostPort(getHostPort(t, server.URL)),
				Options.SynchronousInitialization(true),
			)
			defer throttler.Close()
			throttler.SetProcess(jaeger.Process{UUID: "uuid"})
			assert.True(t, throttler.IsAllowed(testOperation))
			loopUntilCreditsReady(throttler)
			assert.True(t, throttler.IsAllowed(testOperation))
			assert.False(t, throttler.IsAllowed(testOperation))
		})
}

func TestRemotelyControlledThrottler_asynchronousInitialization(t *testing.T) {
	withHTTPServer(
		2,
		func(
			handler *creditHandler,
			server *httptest.Server,
		) {
			throttler := NewThrottler(
				"svc",
				Options.RefreshInterval(time.Millisecond),
				Options.HostPort(getHostPort(t, server.URL)),
			)
			defer throttler.Close()
			throttler.SetProcess(jaeger.Process{UUID: "uuid"})
			assert.False(t, throttler.IsAllowed(testOperation))
			loopUntilCreditsReady(throttler)
			assert.True(t, throttler.IsAllowed(testOperation))
			assert.True(t, throttler.IsAllowed(testOperation))
			assert.False(t, throttler.IsAllowed(testOperation))
		})
}

func loopUntilCreditsReady(throttler *Throttler) {
	for i := 0; i < 1000; i++ {
		throttler.mux.RLock()
		if throttler.credits[testOperation] > 0 {
			throttler.mux.RUnlock()
			break
		}
		throttler.mux.RUnlock()
		time.Sleep(time.Millisecond)
	}
}

func getHostPort(t *testing.T, s string) string {
	u, err := url.Parse(s)
	require.NoError(t, err, "Failed to parse url")
	return u.Host
}
