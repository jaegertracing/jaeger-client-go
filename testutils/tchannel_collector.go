package testutils

import (
	"errors"
	"sync"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/trace/thrift/gen-go/tcollector"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger-client-go/thrift/gen/sampling"
	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"
)

var errTCollectorSamplingNotImplemented = errors.New("TCollector.GetSamplingStrategy method not implemented")

// StartMockTCollector runs a mock representation of Jaeger Collector.
// This function returns a started server, with a Channel that knows
// how to find that server, which can be used in clients or Jaeger tracer.
func StartMockTCollector() (*MockTCollector, error) {
	ch, err := tchannel.NewChannel("tcollector", nil)
	if err != nil {
		return nil, err
	}

	server := thrift.NewServer(ch)

	collector := &MockTCollector{
		Channel:       ch,
		server:        server,
		zipkinSpans:   make([]*zipkincore.Span, 0, 10),
		tchannelSpans: make([]*tcollector.Span, 0, 10),
		samplingMgr:   newSamplingManager(),
	}

	server.Register(zipkincore.NewTChanZipkinCollectorServer(collector))
	server.Register(tcollector.NewTChanTCollectorServer(collector))
	server.Register(sampling.NewTChanSamplingManagerServer(&tchanSamplingManager{collector.samplingMgr}))

	if err := ch.ListenAndServe("127.0.0.1:0"); err != nil {
		return nil, err
	}

	subchannel := ch.GetSubChannel("tcollector", tchannel.Isolated)
	subchannel.Peers().Add(ch.PeerInfo().HostPort)

	return collector, nil
}

// MockTCollector is a mock representation of Jaeger Collector.
type MockTCollector struct {
	Channel       *tchannel.Channel
	server        *thrift.Server
	zipkinSpans   []*zipkincore.Span
	tchannelSpans []*tcollector.Span
	mutex         sync.Mutex
	samplingMgr   *samplingManager
}

// AddSamplingStrategy registers a sampling strategy for a service
func (s *MockTCollector) AddSamplingStrategy(service string, strategy *sampling.SamplingStrategyResponse) {
	s.samplingMgr.AddSamplingStrategy(service, strategy)
}

// GetZipkinSpans returns accumulated Zipkin spans
func (s *MockTCollector) GetZipkinSpans() []*zipkincore.Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.zipkinSpans[:]
}

// GetTChannelSpans returns accumulated TChannel spans
func (s *MockTCollector) GetTChannelSpans() []*tcollector.Span {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.tchannelSpans[:]
}

// Close stops/closes the underlying channel and server
func (s *MockTCollector) Close() {
	s.Channel.Close()
}

// SubmitZipkinBatch implements handler method of TChanZipkinCollectorServer
func (s *MockTCollector) SubmitZipkinBatch(ctx thrift.Context, spans []*zipkincore.Span) ([]*zipkincore.Response, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.zipkinSpans = append(s.zipkinSpans, spans...)
	return []*zipkincore.Response{{Ok: true}}, nil
}

// Submit implements handler method of TChanTCollectorServer
func (s *MockTCollector) Submit(ctx thrift.Context, span *tcollector.Span) (*tcollector.Response, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.tchannelSpans = append(s.tchannelSpans, span)
	return &tcollector.Response{Ok: true}, nil
}

// SubmitBatch implements handler method of TChanTCollectorServer
func (s *MockTCollector) SubmitBatch(ctx thrift.Context, spans []*tcollector.Span) ([]*tcollector.Response, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.tchannelSpans = append(s.tchannelSpans, spans...)
	return []*tcollector.Response{{Ok: true}}, nil
}

// GetSamplingStrategy implements handler method of TChanTCollectorServer
func (s *MockTCollector) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*tcollector.SamplingStrategyResponse, error) {
	return nil, errTCollectorSamplingNotImplemented
}

type tchanSamplingManager struct {
	samplingMgr *samplingManager
}

// GetSamplingStrategy implements GetSamplingStrategy of TChanSamplingManagerServer
func (s *tchanSamplingManager) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return s.samplingMgr.GetSamplingStrategy(serviceName)
}
