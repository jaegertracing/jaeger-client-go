// Copyright (c) 2016 Uber Technologies, Inc.
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

package tchannel

import (
	"time"

	"github.com/uber/tchannel-go"
	"github.com/uber/tchannel-go/thrift"

	"github.com/uber/jaeger-client-go/thrift-gen/zipkincore"
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
