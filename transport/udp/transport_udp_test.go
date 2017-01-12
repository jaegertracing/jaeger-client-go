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

package udp

import (
	"testing"
	"time"

	"github.com/uber/jaeger-client-go/testutils"
	"github.com/uber/jaeger-client-go/thrift-gen/agent"
	"github.com/uber/jaeger-client-go/thrift-gen/zipkincore"

	"github.com/apache/thrift/lib/go/thrift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getThriftSpanByteLength(t *testing.T, span *zipkincore.Span) int {
	transport := thrift.NewTMemoryBufferLen(1000)
	protocolFactory := thrift.NewTCompactProtocolFactory()
	err := span.Write(protocolFactory.GetProtocol(transport))
	require.NoError(t, err)
	return transport.Len()
}

func TestEmitSpanBatchOverhead(t *testing.T) {

	transport := thrift.NewTMemoryBufferLen(1000)
	protocolFactory := thrift.NewTCompactProtocolFactory()
	client := agent.NewAgentClientFactory(transport, protocolFactory)

	span := &zipkincore.Span{Name: "test-span"}
	spanSize := getThriftSpanByteLength(t, span)

	tests := []int{1, 2, 14, 15, 377, 500, 65000, 0xFFFF}
	for i, n := range tests {
		transport.Reset()
		batch := make([]*zipkincore.Span, n)
		for j := 0; j < n; j++ {
			batch[j] = span
		}
		client.SeqId = -2 // this causes the longest encoding of varint32 as 5 bytes
		err := client.EmitZipkinBatch(batch)
		require.NoError(t, err)
		overhead := transport.Len() - n*spanSize
		assert.True(t, overhead <= emitSpanBatchOverhead,
			"test %d, n=%d, expected overhead %d <= %d", i, n, overhead, emitSpanBatchOverhead)
	}
}

func TestUDPSenderFlush(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	span := &zipkincore.Span{Name: "test-span"}
	spanSize := getThriftSpanByteLength(t, span)

	sender, err := NewUDPTransport(agent.SpanServerAddr(), 5*spanSize+emitSpanBatchOverhead)
	require.NoError(t, err)
	udpSender := sender.(*udpSender)

	// test empty flush
	n, err := sender.Flush()
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// test early flush
	n, err = sender.Append(span)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "span should be in buffer, not flushed")
	buffer := udpSender.spanBuffer
	require.Equal(t, 1, len(buffer), "span should be in buffer, not flushed")
	assert.Equal(t, span, buffer[0], "span should be in buffer, not flushed")

	n, err = sender.Flush()
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Equal(t, 0, len(udpSender.spanBuffer), "buffer should become empty")
	assert.Equal(t, 0, udpSender.byteBufferSize, "buffer size counter should go to 0")
	assert.Nil(t, buffer[0], "buffer should not keep reference to the span")

	for i := 0; i < 10000; i++ {
		spans := agent.GetZipkinSpans()
		if len(spans) > 0 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	spans := agent.GetZipkinSpans()
	require.Equal(t, 1, len(spans), "agent should have received the span")
	assert.Equal(t, span.Name, spans[0].Name)
}

func TestUDPSenderAppend(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	span := &zipkincore.Span{Name: "test-span"}
	spanSize := getThriftSpanByteLength(t, span)

	tests := []struct {
		bufferSizeOffset    int
		expectFlush         bool
		expectSpansFlushed  int
		manualFlush         bool
		expectSpansFlushed2 int
		description         string
	}{
		{1, false, 0, true, 5, "in test: buffer bigger than 5 spans"},
		{0, true, 5, false, 0, "in test: buffer fits exactly 5 spans"},
		{-1, true, 4, true, 1, "in test: buffer smaller than 5 spans"},
	}

	for _, test := range tests {
		bufferSize := 5*spanSize + test.bufferSizeOffset + emitSpanBatchOverhead
		sender, err := NewUDPTransport(agent.SpanServerAddr(), bufferSize)
		require.NoError(t, err, test.description)

		agent.ResetZipkinSpans()
		for i := 0; i < 5; i++ {
			n, err := sender.Append(span)
			require.NoError(t, err, test.description)
			if i < 4 {
				assert.Equal(t, 0, n, test.description)
			} else {
				assert.Equal(t, test.expectSpansFlushed, n, test.description)
			}
		}
		if test.expectFlush {
			time.Sleep(5 * time.Millisecond)
		}
		spans := agent.GetZipkinSpans()
		require.Equal(t, test.expectSpansFlushed, len(spans), test.description)
		for i := 0; i < test.expectSpansFlushed; i++ {
			assert.Equal(t, span.Name, spans[i].Name, test.description)
		}

		if test.manualFlush {
			agent.ResetZipkinSpans()
			n, err := sender.Flush()
			require.NoError(t, err, test.description)
			assert.Equal(t, test.expectSpansFlushed2, n, test.description)

			time.Sleep(5 * time.Millisecond)
			spans := agent.GetZipkinSpans()
			require.Equal(t, test.expectSpansFlushed2, len(spans), test.description)
			for i := 0; i < test.expectSpansFlushed2; i++ {
				assert.Equal(t, span.Name, spans[i].Name, test.description)
			}
		}

	}
}

func TestUDPSenderHugeSpan(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	span := &zipkincore.Span{Name: "test-span"}
	spanSize := getThriftSpanByteLength(t, span)

	sender, err := NewUDPTransport(agent.SpanServerAddr(), spanSize/2+emitSpanBatchOverhead)
	require.NoError(t, err)

	n, err := sender.Append(span)
	assert.Equal(t, errSpanTooLarge, err)
	assert.Equal(t, 1, n)
}
