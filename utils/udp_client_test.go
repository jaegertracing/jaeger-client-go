// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-client-go/thrift-gen/zipkincore"
)

func TestNewAgentClientUDPWithParamsBadHostport(t *testing.T) {
	hostPort := "blahblah"

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort: hostPort,
	})

	assert.Error(t, err)
	assert.Nil(t, agentClient)
}

func TestNewAgentClientUDPWithParams(t *testing.T) {
	hostPort := "blahblah:34322"
	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil).
		Once()

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()
	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).
		Once()

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort:      hostPort,
		MaxPacketSize: 25000,
		Logger:        log.NullLogger,
		ResolveFunc:   resolver.ResolveUDPAddr,
		DialFunc:      dialer.DialUDP,
	})
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, 25000, agentClient.maxPacketSize)

	if assert.IsType(t, &resolvedUDPConn{}, agentClient.connUDP) {
		assert.Equal(t, log.NullLogger, agentClient.connUDP.(*resolvedUDPConn).logger)
	}

	assertSockBufferSize(t, 25000, clientConn)
	assertConnWritable(t, clientConn, mockServer)

	assert.NoError(t, agentClient.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestNewAgentClientUDPWithParamsDefaults(t *testing.T) {
	mockServer, clientConn, err := newUDPConn()
	clientConn.Close()
	require.NoError(t, err)
	defer mockServer.Close()

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort: mockServer.LocalAddr().String(),
		Logger:   log.NullLogger,
	})
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, UDPPacketMaxLength, agentClient.maxPacketSize)

	if assert.IsType(t, &net.UDPConn{}, agentClient.connUDP) {
		conn := agentClient.connUDP.(*net.UDPConn)
		assertSockBufferSize(t, UDPPacketMaxLength, conn)
		assertConnWritable(t, conn, mockServer)
	}

	assert.NoError(t, agentClient.Close())
}

func TestNewAgentClientUDPDefaults(t *testing.T) {
	mockServer, clientConn, err := newUDPConn()
	clientConn.Close()
	require.NoError(t, err)
	defer mockServer.Close()

	agentClient, err := NewAgentClientUDP(mockServer.LocalAddr().String(), 25000)
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, 25000, agentClient.maxPacketSize)

	if assert.IsType(t, &net.UDPConn{}, agentClient.connUDP) {
		conn := agentClient.connUDP.(*net.UDPConn)
		assertSockBufferSize(t, 25000, conn)
		assertConnWritable(t, conn, mockServer)
	}

	assert.NoError(t, agentClient.Close())
}

func TestNewAgentClientUDPWithParamsIPHost(t *testing.T) {
	hostPort := "123.123.123.123:34322"
	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil).
		Once()

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()
	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).
		Once()

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort:      hostPort,
		MaxPacketSize: 25000,
		Logger:        log.NullLogger,
		ResolveFunc:   resolver.ResolveUDPAddr,
		DialFunc:      dialer.DialUDP,
	})
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, 25000, agentClient.maxPacketSize)

	assert.IsType(t, &net.UDPConn{}, agentClient.connUDP)

	assertSockBufferSize(t, 25000, clientConn)
	assertConnWritable(t, clientConn, mockServer)

	assert.NoError(t, agentClient.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestNewAgentClientUDPWithParamsIPHostResolveFails(t *testing.T) {
	hostPort := "123.123.123.123:34322"
	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(nil, fmt.Errorf("resolve failed")).
		Once()

	dialer := mockDialer{}

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort:      hostPort,
		MaxPacketSize: 25000,
		Logger:        log.NullLogger,
		ResolveFunc:   resolver.ResolveUDPAddr,
		DialFunc:      dialer.DialUDP,
	})
	assert.Error(t, err)
	assert.Nil(t, agentClient)

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestAgentClientUDPNotSupportZipkin(t *testing.T) {
	agentClient := AgentClientUDP{}

	assert.Error(t, agentClient.EmitZipkinBatch([]*zipkincore.Span{{Name: "fakespan"}}))
}
