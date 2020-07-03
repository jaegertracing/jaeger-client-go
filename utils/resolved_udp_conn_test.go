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
	"fmt"
	"net"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-client-go/log"
)

type mockResolver struct {
	MockHost string
	mock.Mock
}

func (m *mockResolver) ResolveUDPAddr(network string, hostPort string) (*net.UDPAddr, error) {
	args := m.Called(network, hostPort)

	a0 := args.Get(0)
	if a0 == nil {
		return (*net.UDPAddr)(nil), args.Error(1)
	}
	return a0.(*net.UDPAddr), args.Error(1)
}

type mockDialer struct {
	mock.Mock
}

func (m *mockDialer) DialUDP(network string, laddr, raddr *net.UDPAddr) (*net.UDPConn, error) {
	args := m.Called(network, laddr, raddr)

	a0 := args.Get(0)
	if a0 == nil {
		return (*net.UDPConn)(nil), args.Error(1)
	}

	return a0.(*net.UDPConn), args.Error(1)
}

func newUDPConn() (net.PacketConn, *net.UDPConn, error) {
	mockServer, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}

	addr, err := net.ResolveUDPAddr("udp", mockServer.LocalAddr().String())
	if err != nil {
		mockServer.Close()
		return nil, nil, err
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		mockServer.Close()
		return nil, nil, err
	}

	return mockServer, conn, nil
}

func assertSockBufferSize(t *testing.T, expectedBytes int, conn *net.UDPConn) bool {
	fd, _ := conn.File()
	bufferBytes, _ := syscall.GetsockoptInt(int(fd.Fd()), syscall.SOL_SOCKET, syscall.SO_SNDBUF)

	// The linux kernel doubles SO_SNDBUF value (to allow space for bookkeeping overhead) when it is set using setsockopt(2), and this doubled value is returned by getsockopt(2)
	// https://linux.die.net/man/7/socket
	if runtime.GOOS == "linux" {
		return assert.GreaterOrEqual(t, expectedBytes*2, bufferBytes)
	} else {
		return assert.Equal(t, expectedBytes, bufferBytes)
	}
}

func assertConnWritable(t *testing.T, conn udpConn, serverConn net.PacketConn) (allSucceed bool) {
	expectedString := "yo this is a test"
	_, err := conn.Write([]byte(expectedString))
	allSucceed = assert.NoError(t, err)

	var buf = make([]byte, len(expectedString))
	err = serverConn.SetReadDeadline(time.Now().Add(time.Second))
	assertion := assert.NoError(t, err)
	allSucceed = allSucceed && assertion

	_, _, err = serverConn.ReadFrom(buf)
	assertion = assert.NoError(t, err)
	allSucceed = allSucceed && assertion
	assertion = assert.Equal(t, []byte(expectedString), buf)

	return allSucceed && assertion
}

func TestNewResolvedUDPConn(t *testing.T) {
	hostPort := "blahblah:34322"

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()

	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil).
		Once()

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).
		Once()

	conn, err := newResolvedUDPConn(hostPort, time.Hour, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	require.NotNil(t, conn)

	err = conn.Close()
	assert.NoError(t, err)

	// assert the actual connection was closed
	assert.Error(t, clientConn.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestResolvedUDPConnWrites(t *testing.T) {
	hostPort := "blahblah:34322"

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()

	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil).
		Once()

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).
		Once()

	conn, err := newResolvedUDPConn(hostPort, time.Hour, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	require.NotNil(t, conn)

	assertConnWritable(t, conn, mockServer)

	err = conn.Close()
	assert.NoError(t, err)

	// assert the actual connection was closed
	assert.Error(t, clientConn.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestResolvedUDPConnEventuallyDials(t *testing.T) {
	hostPort := "blahblah:34322"

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()

	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(nil, fmt.Errorf("failed to resolve")).Once().
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil)

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).Once()

	conn, err := newResolvedUDPConn(hostPort, time.Millisecond*10, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	require.NotNil(t, conn)

	err = conn.SetWriteBuffer(UDPPacketMaxLength)
	assert.NoError(t, err)

	for i := 0; i < 10; i++ {
		conn.connMtx.RLock()
		connected := conn.conn != nil
		conn.connMtx.RUnlock()
		if connected {
			break
		}
		time.Sleep(time.Millisecond * 10)
	}

	conn.connMtx.RLock()
	assert.NotNil(t, conn.conn)
	conn.connMtx.RUnlock()

	assertConnWritable(t, conn, mockServer)
	assertSockBufferSize(t, UDPPacketMaxLength, clientConn)

	err = conn.Close()
	assert.NoError(t, err)

	// assert the actual connection was closed
	assert.Error(t, clientConn.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestResolvedUDPConnNoSwapIfFail(t *testing.T) {
	hostPort := "blahblah:34322"

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()

	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil).Once()

	failCalled := make(chan struct{})
	resolver.On("ResolveUDPAddr", "udp", hostPort).
		Run(func(args mock.Arguments) {
			close(failCalled)
		}).
		Return(nil, fmt.Errorf("resolve failed"))

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).Once()

	conn, err := newResolvedUDPConn(hostPort, time.Millisecond*10, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	require.NotNil(t, conn)

	var wasCalled bool
	// wait at most 100 milliseconds for the second call of ResolveUDPAddr that is supposed to fail
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
	select {
	case <-failCalled:
		wasCalled = true
	case <-ctx.Done():
	}
	cancel()

	assert.True(t, wasCalled)

	assertConnWritable(t, conn, mockServer)

	err = conn.Close()
	assert.NoError(t, err)

	// assert the actual connection was closed
	assert.Error(t, clientConn.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}

func TestResolvedUDPConnWriteRetry(t *testing.T) {
	hostPort := "blahblah:34322"

	mockServer, clientConn, err := newUDPConn()
	require.NoError(t, err)
	defer mockServer.Close()

	resolver := mockResolver{}
	resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(nil, fmt.Errorf("failed to resolve")).Once().
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil).Once()

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).Once()

	conn, err := newResolvedUDPConn(hostPort, time.Millisecond*10, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	require.NotNil(t, conn)

	err = conn.SetWriteBuffer(UDPPacketMaxLength)
	assert.NoError(t, err)

	assertConnWritable(t, conn, mockServer)
	assertSockBufferSize(t, UDPPacketMaxLength, clientConn)

	err = conn.Close()
	assert.NoError(t, err)

	// assert the actual connection was closed
	assert.Error(t, clientConn.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}
