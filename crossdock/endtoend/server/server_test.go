package server

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-client-go/config"
)

var (
	testOperation = "testOperation"

	testTraceRequest = traceRequest{
		Operation: testOperation,
		Tags: map[string]string{
			"key": "value",
		},
		Count: 2,
	}

	testInitRequest = initRequest{
		Service: "testService",
		Sampler: &config.SamplerConfig{
			Type:               "remote",
			Param:              1,
			LocalAgentHostPort: "localhost:1000",
		},
		Reporter: &reporterConfig{
			BufferFlushInterval: 1,
			LocalAgentHostPort:  "localhost:2000",
		},
	}

	testInitJSONRequest = `
		{
			"service": "testService",
			"sampler": {
				"type": "remote",
				"param": 1,
				"localAgentHostPort": "localhost:5778"
			},
			"reporter": {
				"bufferFlushInterval": 1,
				"localAgentHostPort": "localhost:5775"
			}
		}
	`

	testInvalidJSON = `bad_json`

	testBadInitJSONRequest = `
		{
			"reporter": {
				"bufferFlushInterval": 1,
				"localAgentHostPort": "localhost:5775"
			}
		}
	`

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

func TestInit(t *testing.T) {
	tests := []struct {
		expectedCode int
		json         string
		tracerIsNil  bool
	}{
		{http.StatusOK, testInitJSONRequest, false},
		{http.StatusBadRequest, testInvalidJSON, true},
		{http.StatusInternalServerError, testBadInitJSONRequest, true},
	}

	for _, test := range tests {
		req, err := http.NewRequest("POST", "/init", bytes.NewBuffer([]byte(test.json)))
		if err != nil {
			assert.FailNow(t, "Failed to initialize request: %v", err)
		}
		recorder := httptest.NewRecorder()
		handler := &Handler{}
		handlerFunc := http.HandlerFunc(handler.Init)

		handlerFunc.ServeHTTP(recorder, req)

		assert.Equal(t, test.expectedCode, recorder.Code)
		if test.tracerIsNil {
			assert.Nil(t, handler.tracer)
		} else {
			assert.NotNil(t, handler.tracer)
		}
	}
}

func TestInitializeTracer(t *testing.T) {
	handler := &Handler{}
	err := handler.initializeTracer(&testInitRequest)
	assert.NoError(t, err)
}

func TestTrace(t *testing.T) {
	tests := []struct {
		expectedCode int
		json         string
		handler      *Handler
	}{
		{http.StatusOK, testTraceJSONRequest, &Handler{tracer: newMockTracer()}},
		{http.StatusBadRequest, testInvalidJSON, &Handler{}},
		{http.StatusBadRequest, testTraceJSONRequest, &Handler{}}, // Tracer hasn't been initialized
	}

	for _, test := range tests {
		req, err := http.NewRequest("POST", "/trace", bytes.NewBuffer([]byte(test.json)))
		if err != nil {
			assert.FailNow(t, "Failed to initialize request: %v", err)
		}
		recorder := httptest.NewRecorder()
		handlerFunc := http.HandlerFunc(test.handler.Trace)

		handlerFunc.ServeHTTP(recorder, req)

		assert.Equal(t, test.expectedCode, recorder.Code)
	}
}

func TestGenerateTraces(t *testing.T) {
	mTracer := newMockTracer()
	handler := &Handler{tracer: mTracer}
	handler.generateTraces(&testTraceRequest)
	assert.Equal(t, 2, mTracer.count[testOperation])
}
