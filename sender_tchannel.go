package jaeger

import (
	"time"

	z "github.com/uber/jaeger-client-go/thrift/gen/zipkincore"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"
)

type tchannelSender struct {
	client    z.TChanZipkinCollector
	batchSize int
	buffer    []*z.Span
}

// NewTChannelSender creates a reporter that submits spans to a TChannel service `serviceName`
func NewTChannelSender(ch *tchannel.Channel, serviceName string, batchSize int) Sender {
	if serviceName == "" {
		serviceName = defaultTChannelServiceName
	}
	if batchSize == 0 {
		batchSize = defaultBatchSize
	}

	thriftClient := thrift.NewClient(ch, serviceName, nil)
	client := z.NewTChanZipkinCollectorClient(thriftClient)
	return &tchannelSender{
		client:    client,
		batchSize: batchSize,
		buffer:    make([]*z.Span, 0, batchSize)}
}

// Append implements Append() of Sender
func (s *tchannelSender) Append(span *z.Span) (int, error) {
	s.buffer = append(s.buffer, span)
	if len(s.buffer) >= s.batchSize {
		return s.Flush()
	}
	return 0, nil
}

// Flush implements Flush() of Sender
func (s *tchannelSender) Flush() (int, error) {
	if len(s.buffer) == 0 {
		return 0, nil
	}

	// It would be nice if TChannel allowed a context without a timeout.
	// We could've reused the same context instead of always allocating.
	ctx, cancel := tchannel.NewContextBuilder(5 * time.Second).DisableTracing().Build()
	defer cancel()

	_, err := s.client.SubmitZipkinBatch(ctx, s.buffer)

	for i := range s.buffer {
		s.buffer[i] = nil
	}
	flushed := len(s.buffer)
	s.buffer = s.buffer[:0]

	return flushed, err
}

// Close implements Close() of Sender. Does nothing, since we do not own the TChannel.
func (s *tchannelSender) Close() error {
	return nil
}
