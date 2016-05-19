package utils

import (
	"fmt"
	"io"
	"net"

	"github.com/uber/jaeger-client-go/thrift/gen/agent"
	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"

	"github.com/apache/thrift/lib/go/thrift"
)

// UDPPacketMaxLength is the max size of UDP packet we want to send, synced with jaeger-agent
const UDPPacketMaxLength = 65000

// AgentClientUDP is a UDP client to Jaeger agent that implements agent.Agent interface.
type AgentClientUDP struct {
	agent.Agent
	io.Closer

	connUDP       *net.UDPConn
	client        *agent.AgentClient
	maxPacketSize int                   // max size of datagram in bytes
	thriftBuffer  *thrift.TMemoryBuffer // buffer used to calculate byte size of a span
}

// NewAgentClientUDP creates a client that sends spans to Jaeger Agent over UDP.
func NewAgentClientUDP(hostPort string, maxPacketSize int) (*AgentClientUDP, error) {
	if maxPacketSize == 0 {
		maxPacketSize = UDPPacketMaxLength
	}

	thriftBuffer := thrift.NewTMemoryBufferLen(maxPacketSize)
	protocolFactory := thrift.NewTCompactProtocolFactory()
	client := agent.NewAgentClientFactory(thriftBuffer, protocolFactory)

	destAddr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return nil, err
	}

	connUDP, err := net.DialUDP(destAddr.Network(), nil, destAddr)
	if err != nil {
		return nil, err
	}
	if err := connUDP.SetWriteBuffer(maxPacketSize); err != nil {
		return nil, err
	}

	clientUDP := &AgentClientUDP{
		connUDP:       connUDP,
		client:        client,
		maxPacketSize: maxPacketSize,
		thriftBuffer:  thriftBuffer}
	return clientUDP, nil
}

// EmitZipkinBatch implements EmitZipkinBatch() of Agent interface
func (a *AgentClientUDP) EmitZipkinBatch(spans []*zipkincore.Span) error {
	a.thriftBuffer.Reset()
	a.client.SeqId = 0 // we have no need for distinct SeqIds for our one-way UDP messages
	if err := a.client.EmitZipkinBatch(spans); err != nil {
		return err
	}
	if a.thriftBuffer.Len() > a.maxPacketSize {
		return fmt.Errorf("Data does not fit within one UDP packet; size %d, max %d, spans %d",
			a.thriftBuffer.Len(), a.maxPacketSize, len(spans))
	}
	_, err := a.connUDP.Write(a.thriftBuffer.Bytes())
	return err
}

// Close implements Close() of io.Closer and closes the underlying UDP connection.
func (a *AgentClientUDP) Close() error {
	return a.connUDP.Close()
}
