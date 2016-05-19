package tchannel

import (
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"
	"github.com/uber/jaeger-client-go/transport"
)

const (
	defaultTChannelServiceName = "tcollector"
	defaultBatchSize           = 10
)

// Transport submits Thrift spans over TChannel
type Transport struct {
	Client    zipkincore.TChanZipkinCollector
	batchSize int
	buffer    []*zipkincore.Span
}

// New creates a reporter that submits spans to a TChannel service `serviceName`
func New(ch *tchannel.Channel, serviceName string, batchSize int) transport.Transport {
	if serviceName == "" {
		serviceName = defaultTChannelServiceName
	}
	if batchSize == 0 {
		batchSize = defaultBatchSize
	}

	thriftClient := thrift.NewClient(ch, serviceName, nil)
	client := zipkincore.NewTChanZipkinCollectorClient(thriftClient)
	return &Transport{
		Client:    client,
		batchSize: batchSize,
		buffer:    make([]*zipkincore.Span, 0, batchSize)}
}

// Append implements Append() of Sender
func (s *Transport) Append(span *zipkincore.Span) (int, error) {
	s.buffer = append(s.buffer, span)
	if len(s.buffer) >= s.batchSize {
		return s.Flush()
	}
	return 0, nil
}

// Flush implements Flush() of Sender
func (s *Transport) Flush() (int, error) {
	if len(s.buffer) == 0 {
		return 0, nil
	}

	// It would be nice if TChannel allowed a context without a timeout.
	// We could've reused the same context instead of always allocating.
	ctx, cancel := tchannel.NewContextBuilder(5 * time.Second).DisableTracing().Build()
	defer cancel()

	_, err := s.Client.SubmitZipkinBatch(ctx, s.buffer)

	for i := range s.buffer {
		s.buffer[i] = nil
	}
	flushed := len(s.buffer)
	s.buffer = s.buffer[:0]

	return flushed, err
}

// Close implements Close() of Sender. Does nothing, since we do not own the TChannel.
func (s *Transport) Close() error {
	return nil
}
