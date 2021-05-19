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
	"context"
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
	mockServer, err := newUDPListener()
	require.NoError(t, err)
	defer mockServer.Close()

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort:      mockServer.LocalAddr().String(),
		MaxPacketSize: 25000,
		Logger:        log.NullLogger,
	})
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, 25000, agentClient.maxPacketSize)

	if assert.IsType(t, &reconnectingUDPConn{}, agentClient.connUDP) {
		assert.Equal(t, log.NullLogger, agentClient.connUDP.(*reconnectingUDPConn).logger)
	}

	assert.NoError(t, agentClient.Close())
}

func TestNewAgentClientUDPWithParamsDefaults(t *testing.T) {
	mockServer, err := newUDPListenerOnPort(6831)
	require.NoError(t, err)
	defer mockServer.Close()

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort: "localhost:6831",
	})
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, UDPPacketMaxLength, agentClient.maxPacketSize)

	if assert.IsType(t, &reconnectingUDPConn{}, agentClient.connUDP) {
		assert.Equal(t, agentClient.connUDP.(*reconnectingUDPConn).logger, log.StdLogger)
	}

	assert.NoError(t, agentClient.Close())
}

func TestNewAgentClientUDPDefaults(t *testing.T) {
	mockServer, err := newUDPListenerOnPort(6831)
	require.NoError(t, err)
	defer mockServer.Close()

	agentClient, err := NewAgentClientUDP("localhost:6831", 0)
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, UDPPacketMaxLength, agentClient.maxPacketSize)

	if assert.IsType(t, &reconnectingUDPConn{}, agentClient.connUDP) {
		assert.Equal(t, agentClient.connUDP.(*reconnectingUDPConn).logger, log.StdLogger)
	}

	assert.NoError(t, agentClient.Close())
}

func TestNewAgentClientUDPWithParamsReconnectingDisabled(t *testing.T) {
	mockServer, err := newUDPListener()
	require.NoError(t, err)
	defer mockServer.Close()

	agentClient, err := NewAgentClientUDPWithParams(AgentClientUDPParams{
		HostPort:                   mockServer.LocalAddr().String(),
		Logger:                     log.NullLogger,
		DisableAttemptReconnecting: true,
	})
	assert.NoError(t, err)
	assert.NotNil(t, agentClient)
	assert.Equal(t, UDPPacketMaxLength, agentClient.maxPacketSize)

	assert.IsType(t, &net.UDPConn{}, agentClient.connUDP)

	assert.NoError(t, agentClient.Close())
}

func TestAgentClientUDPNotSupportZipkin(t *testing.T) {
	agentClient := AgentClientUDP{}

	assert.Error(t, agentClient.EmitZipkinBatch(context.Background(), []*zipkincore.Span{{Name: "fakespan"}}))
}
