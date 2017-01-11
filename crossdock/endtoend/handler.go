package endtoend

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/opentracing/opentracing-go"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/crossdock/common"
)

var (
	// EndToEndConfig is the default config used to connect tracer to other crossdock components
	EndToEndConfig = config.Configuration{
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
	tracer opentracing.Tracer
}

type traceRequest struct {
	Operation string            `json:"operation"`
	Tags      map[string]string `json:"tags"`
	Count     int               `json:"count"`
}

// Init initializes the handler with a tracer
func (h *Handler) Init(cfg config.Configuration) error {
	tracer, _, err := cfg.New(common.DefaultTracerServiceName, jaeger.NullStatsReporter)
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
