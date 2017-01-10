package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
)

// Handler handles creating traces via jaeger-client.
type Handler struct {
	tracer opentracing.Tracer
}

type reporterConfig struct {
	BufferFlushInterval int    `json:"bufferFlushInterval"`
	LocalAgentHostPort  string `json:"localAgentHostPort"`
}

type initRequest struct {
	Service  string                `json:"service"`
	Sampler  *config.SamplerConfig `json:"sampler"`
	Reporter *reporterConfig       `json:"reporter"`
}

type traceRequest struct {
	Operation string            `json:"operation"`
	Tags      map[string]string `json:"tags"`
	Count     int               `json:"count"`
}

// Init initializes the jaeger client given the parameters in the request.
func (h *Handler) Init(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var req initRequest
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("JSON payload is invalid: %s", err.Error()), http.StatusBadRequest)
		return
	}
	if err := h.initializeTracer(&req); err != nil {
		http.Error(w, fmt.Sprintf("Could not initialize tracer: %s", err.Error()), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) initializeTracer(req *initRequest) error {
	cfg := config.Configuration{
		Disabled: false,
		Sampler:  req.Sampler,
		Reporter: &config.ReporterConfig{
			BufferFlushInterval: time.Duration(req.Reporter.BufferFlushInterval) * time.Second,
			LocalAgentHostPort:  req.Reporter.LocalAgentHostPort,
		},
	}
	tracer, _, err := cfg.New(req.Service, jaeger.NullStatsReporter)
	if err != nil {
		return err
	}
	h.tracer = tracer
	return nil
}

// Trace creates traces given the parameters in the request.
func (h *Handler) Trace(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	var req traceRequest
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "JSON payload is invalid", http.StatusBadRequest)
		return
	}
	if h.tracer == nil {
		http.Error(w, "Call init before trace", http.StatusBadRequest)
		return
	}
	h.generateTraces(&req)
}

func (h *Handler) generateTraces(r *traceRequest) {
	opts := make([]opentracing.StartSpanOption, 0, len(r.Tags))
	for k, v := range r.Tags {
		opts = append(opts, opentracing.Tag{Key: k, Value: v})
	}
	for i := 0; i < r.Count; i++ {
		span := h.tracer.StartSpan(r.Operation, opts...)
		span.Finish()
	}
}
