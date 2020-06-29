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
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/uber/jaeger-client-go/log"
)

type mockResolver struct {
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

func TestNewResolvedUDPConn(t *testing.T) {
	hostPort := "blahblah:34322"

	mockServer, clientConn, err := newUDPConn()
	if !assert.NoError(t, err) {
		t.FailNow()
	}
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
	assert.NotNil(t, conn)

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
	if !assert.NoError(t, err) {
		t.FailNow()
	}
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
	assert.NotNil(t, conn)

	expectedString := "yo this is a test"
	_, err = conn.Write([]byte(expectedString))
	assert.NoError(t, err)

	var buf = make([]byte, len(expectedString))
	err = mockServer.SetReadDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)
	_, _, err = mockServer.ReadFrom(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte(expectedString), buf)

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
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	defer mockServer.Close()

	resolver := mockResolver{}
	resolveCall := resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(nil, fmt.Errorf("failed to resolve"))

	timer := time.AfterFunc(time.Millisecond*100, func() {
		resolveCall.Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil)
	})
	defer timer.Stop()

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).
		Once()

	conn, err := newResolvedUDPConn(hostPort, time.Millisecond*100, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	err = conn.SetWriteBuffer(65000)
	assert.NoError(t, err)

	<-time.After(time.Millisecond * 200)

	fd, _ := clientConn.File()
	bufferBytes, _ := syscall.GetsockoptInt(int(fd.Fd()), syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if runtime.GOOS == "darwin" {
		assert.Equal(t, 65000, bufferBytes)
	} else {
		assert.GreaterOrEqual(t, 65000*2, bufferBytes)
	}

	expectedString := "yo this is a test"
	_, err = conn.Write([]byte(expectedString))
	assert.NoError(t, err)

	var buf = make([]byte, len(expectedString))
	err = mockServer.SetReadDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)
	_, _, err = mockServer.ReadFrom(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte(expectedString), buf)

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
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	defer mockServer.Close()

	resolver := mockResolver{}
	resolveCall := resolver.
		On("ResolveUDPAddr", "udp", hostPort).
		Return(&net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}, nil)

	// Simulate resolve fail after 100 seconds to ensure connection isn't swapped
	timer := time.AfterFunc(time.Millisecond*100, func() {
		resolveCall.Return(nil, fmt.Errorf("resolve failed"))
	})
	defer timer.Stop()

	dialer := mockDialer{}
	dialer.
		On("DialUDP", "udp", (*net.UDPAddr)(nil), &net.UDPAddr{
			IP:   net.IPv4(123, 123, 123, 123),
			Port: 34322,
		}).
		Return(clientConn, nil).
		Once()

	conn, err := newResolvedUDPConn(hostPort, time.Millisecond*100, resolver.ResolveUDPAddr, dialer.DialUDP, log.NullLogger)
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	<-time.After(time.Millisecond * 200)
	expectedString := "yo this is a test"
	_, err = conn.Write([]byte(expectedString))
	assert.NoError(t, err)

	var buf = make([]byte, len(expectedString))
	err = mockServer.SetReadDeadline(time.Now().Add(time.Second))
	assert.NoError(t, err)
	_, _, err = mockServer.ReadFrom(buf)
	assert.NoError(t, err)
	assert.Equal(t, []byte(expectedString), buf)

	err = conn.Close()
	assert.NoError(t, err)

	// assert the actual connection was closed
	assert.Error(t, clientConn.Close())

	resolver.AssertExpectations(t)
	dialer.AssertExpectations(t)
}
