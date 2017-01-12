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

package endtoend

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"

	"fmt"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/crossdock/common"
)

var (
	endToEndConfig = config.Configuration{
		Disabled: false,
		Sampler: &config.SamplerConfig{
			Type:               jaeger.SamplerTypeRemote,
			Param:              1.0,
			LocalAgentHostPort: "test_driver:5778",
		},
		Reporter: &config.ReporterConfig{
			BufferFlushInterval: time.Second,
			LocalAgentHostPort:  "test_driver:5775",
		},
	}
)

// Handler creates traces via jaeger-client.
type Handler struct {
	sync.RWMutex

	tracer opentracing.Tracer
}

type traceRequest struct {
	Operation string            `json:"operation"`
	Tags      map[string]string `json:"tags"`
	Count     int               `json:"count"`
}

// init initializes the handler with a tracer
func (h *Handler) init(cfg config.Configuration) error {
	tracer, _, err := cfg.New(common.DefaultTracerServiceName, jaeger.NullStatsReporter)
	if err != nil {
		return err
	}
	h.tracer = tracer
	return nil
}

// GenerateTraces creates traces given the parameters in the request.
func (h *Handler) GenerateTraces(w http.ResponseWriter, r *http.Request) {
	h.Lock()
	if h.tracer == nil {
		h.init(endToEndConfig)
	}
	h.Unlock()
	if h.tracer == nil {
		http.Error(w, "Tracer is not initialized", http.StatusInternalServerError)
		return
	}
	decoder := json.NewDecoder(r.Body)
	var req traceRequest
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("JSON payload is invalid: %s", err.Error()), http.StatusBadRequest)
		return
	}
	h.generateTraces(&req)
}

func (h *Handler) generateTraces(r *traceRequest) {
	for i := 0; i < r.Count; i++ {
		span := h.tracer.StartSpan(r.Operation)
		for k, v := range r.Tags {
			span.SetTag(k, v)
		}
		span.Finish()
	}
}
