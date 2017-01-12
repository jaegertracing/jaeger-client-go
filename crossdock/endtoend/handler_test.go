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
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
)

var (
	testOperation = "testOperation"
	testService   = "testService"

	testConfig = config.Configuration{
		Disabled: false,
		Sampler: &config.SamplerConfig{
			Type:               jaeger.SamplerTypeRemote,
			Param:              1.0,
			LocalAgentHostPort: "localhost:5778",
		},
		Reporter: &config.ReporterConfig{
			BufferFlushInterval: time.Second,
			LocalAgentHostPort:  "localhost:5775",
		},
	}

	badConfig = config.Configuration{
		Disabled: false,
		Sampler: &config.SamplerConfig{
			Type: "INVALID_TYPE",
		},
	}

	testTraceRequest = traceRequest{
		Operation: testOperation,
		Tags: map[string]string{
			"key": "value",
		},
		Count: 2,
	}

	testInvalidJSON = `bad_json`

	testTraceJSONRequest = `
		{
			"operation": "testOperation",
			"tags": {
				"key": "value"
			},
			"count": 2
		}
	`
)

func newInMemoryTracer() (opentracing.Tracer, *jaeger.InMemoryReporter) {
	inMemoryReporter := jaeger.NewInMemoryReporter()
	tracer, _ := jaeger.NewTracer(
		testService,
		jaeger.NewConstSampler(true),
		inMemoryReporter,
		jaeger.TracerOptions.Metrics(jaeger.NewMetrics(jaeger.NullStatsReporter, nil)),
		jaeger.TracerOptions.Logger(jaeger.NullLogger))
	return tracer, inMemoryReporter
}

func TestInit(t *testing.T) {
	handler := &Handler{}
	err := handler.init(testConfig)
	assert.NoError(t, err)
}

func TestInitBadConfig(t *testing.T) {
	handler := &Handler{}
	err := handler.init(badConfig)
	assert.Error(t, err)
}

func TestGenerateTraces(t *testing.T) {
	tracer, _ := newInMemoryTracer()

	tests := []struct {
		expectedCode int
		json         string
		handler      *Handler
	}{
		{http.StatusOK, testTraceJSONRequest, &Handler{tracer: tracer}},
		{http.StatusBadRequest, testInvalidJSON, &Handler{tracer: tracer}},
		{http.StatusInternalServerError, testTraceJSONRequest, &Handler{}}, // Tracer hasn't been initialized
	}

	for _, test := range tests {
		req, err := http.NewRequest("POST", "/create_spans", bytes.NewBuffer([]byte(test.json)))
		assert.NoError(t, err, "Failed to initialize request")
		recorder := httptest.NewRecorder()
		handlerFunc := http.HandlerFunc(test.handler.GenerateTraces)
		handlerFunc.ServeHTTP(recorder, req)

		assert.Equal(t, test.expectedCode, recorder.Code)
	}
}

func TestGenerateTracesInternal(t *testing.T) {
	tracer, reporter := newInMemoryTracer()
	handler := &Handler{tracer: tracer}
	handler.generateTraces(&testTraceRequest)
	assert.Equal(t, 2, reporter.SpansSubmitted())
}
