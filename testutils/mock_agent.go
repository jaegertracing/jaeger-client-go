package testutils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"

	"github.com/uber/jaeger-client-go/thrift/gen/agent"
	"github.com/uber/jaeger-client-go/thrift/gen/sampling"
	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"
	"github.com/uber/jaeger-client-go/utils"

	"github.com/apache/thrift/lib/go/thrift"
)

// StartMockAgent runs a mock representation of jaeger-agent.
// This function returns a started server.
func StartMockAgent() (*MockAgent, error) {
	transport, err := NewTUDPServerTransport(":0")
	if err != nil {
		return nil, err
	}

	samplingManager := newSamplingManager()
	samplingHandler := &samplingHandler{manager: samplingManager}
	samplingServer := httptest.NewServer(samplingHandler)

	agent := &MockAgent{
		transport:   transport,
		samplingMgr: samplingManager,
		samplingSrv: samplingServer,
	}

	var started sync.WaitGroup
	started.Add(1)
	go agent.serve(&started)
	started.Wait()

	return agent, nil
}

// Close stops the serving of traffic
func (s *MockAgent) Close() {
	atomic.StoreUint32(&s.serving, 0)
	s.transport.Close()
	s.samplingSrv.Close()
}

// MockAgent is a mock representation of Jaeger Agent.
// It receives spans over UDP, and has an HTTP endpoint for sampling strategies.
type MockAgent struct {
	transport   *TUDPTransport
	zipkinSpans []*zipkincore.Span
	mutex       sync.Mutex
	serving     uint32
	samplingMgr *samplingManager
	samplingSrv *httptest.Server
}

// SpanServerAddr returns the UDP host:port where MockAgent listens for spans
func (s *MockAgent) SpanServerAddr() string {
	return s.transport.Addr().String()
}

// SpanServerClient returns a UDP client that can be used to send spans to the MockAgent
func (s *MockAgent) SpanServerClient() (agent.Agent, error) {
	return utils.NewAgentClientUDP(s.SpanServerAddr(), 0)
}

// SamplingServerAddr returns the host:port of HTTP server exposing sampling strategy endpoint
func (s *MockAgent) SamplingServerAddr() string {
	return s.samplingSrv.Listener.Addr().String()
}

func (s *MockAgent) serve(started *sync.WaitGroup) {
	handler := agent.NewAgentProcessor(s)
	protocolFact := thrift.NewTCompactProtocolFactory()
	buf := make([]byte, utils.UDPPacketMaxLength, utils.UDPPacketMaxLength)
	trans := thrift.NewTMemoryBufferLen(utils.UDPPacketMaxLength)

	atomic.StoreUint32(&s.serving, 1)
	started.Done()
	for s.IsServing() {
		n, err := s.transport.Read(buf)
		if err == nil {
			trans.Write(buf[:n])
			protocol := protocolFact.GetProtocol(trans)
			handler.Process(protocol, protocol)
		}
	}
}

// EmitZipkinBatch implements EmitZipkinBatch() of TChanSamplingManagerServer
func (s *MockAgent) EmitZipkinBatch(spans []*zipkincore.Span) (err error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.zipkinSpans = append(s.zipkinSpans, spans...)
	return err
}

// IsServing indicates whether the server is currently serving traffic
func (s *MockAgent) IsServing() bool {
	return atomic.LoadUint32(&s.serving) == 1
}

// AddSamplingStrategy registers a sampling strategy for a service
func (s *MockAgent) AddSamplingStrategy(service string, strategy *sampling.SamplingStrategyResponse) {
	s.samplingMgr.AddSamplingStrategy(service, strategy)
}

// GetZipkinSpans returns accumulated Zipkin spans
func (s *MockAgent) GetZipkinSpans() []*zipkincore.Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	n := len(s.zipkinSpans)
	spans := make([]*zipkincore.Span, n, n)
	copy(spans, s.zipkinSpans)
	return spans
}

// ResetZipkinSpans discards accumulated Zipkin spans
func (s *MockAgent) ResetZipkinSpans() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.zipkinSpans = nil
}

type samplingHandler struct {
	manager *samplingManager
}

func (h *samplingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	services := r.URL.Query()["service"]
	if len(services) == 0 {
		http.Error(w, "'service' parameter is empty", http.StatusBadRequest)
		return
	}
	if len(services) > 1 {
		http.Error(w, "'service' parameter must occur only once", http.StatusBadRequest)
		return
	}
	resp, err := h.manager.GetSamplingStrategy(services[0])
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving strategy: %+v", err), http.StatusInternalServerError)
		return
	}
	json, err := json.Marshal(resp)
	if err != nil {
		http.Error(w, "Cannot marshall Thrift to JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	if _, err := w.Write(json); err != nil {
		return
	}
}
