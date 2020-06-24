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
	"github.com/uber/jaeger-client-go/log"
	"net"
	"sync"
	"time"
)

// ResolvedUDPConn is an implementation of udpConn that continually resolves the provided hostname and atomically swaps
// the connection if the dial succeeds
type ResolvedUDPConn struct {
	hostPort    string
	resolveFunc resolveFunc
	dialFunc    dialFunc
	logger      log.Logger

	connMtx   sync.RWMutex
	conn      *net.UDPConn
	destAddr  *net.UDPAddr
	closeChan chan struct{}
	doneChan  chan struct{}
}

type resolveFunc func(network string, hostPort string) (*net.UDPAddr, error)
type dialFunc func(network string, laddr, raddr *net.UDPAddr) (*net.UDPConn, error)

// NewResolvedUDPConn returns a new udpConn that resolves hostPort every resolveTimeout, if the resolved address is
// different than the current conn then the new address is dial and the conn is swapped
func NewResolvedUDPConn(hostPort string, resolveTimeout time.Duration, resolveFunc resolveFunc, dialFunc dialFunc, logger log.Logger) (*ResolvedUDPConn, error) {
	conn := &ResolvedUDPConn{
		hostPort:    hostPort,
		resolveFunc: resolveFunc,
		dialFunc:    dialFunc,
		logger:      logger,
	}

	err := conn.attemptResolveAndDial()
	if err != nil {
		return nil, err
	}

	conn.closeChan = make(chan struct{})
	conn.doneChan = make(chan struct{})

	go conn.resolveLoop(resolveTimeout)

	return conn, nil
}

func (c *ResolvedUDPConn) resolveLoop(resolveTimeout time.Duration) {
	ticker := time.NewTicker(resolveTimeout)
	defer ticker.Stop()

	defer func() {
		if c.doneChan != nil {
			c.doneChan <- struct{}{}
		}
	}()

	for {
		select {
		case <-c.closeChan:
			return
		case <-ticker.C:
			if err := c.attemptResolveAndDial(); err != nil {
				c.logger.Error(err.Error())
			}
		}
	}
}

func (c *ResolvedUDPConn) attemptResolveAndDial() error {
	newAddr, err := c.resolveFunc("udp", c.hostPort)
	if err != nil {
		return fmt.Errorf("failed to resolve new addr, with err: %w", err)
	}

	// dont attempt dial if resolved addr is the same as current conn
	if c.destAddr != nil && newAddr.String() == c.destAddr.String() {
		return nil
	}

	if err := c.attemptDialNewAddr(newAddr); err != nil {
		return fmt.Errorf("failed to dial newly resolved addr %s, with err: %v", newAddr.String(), err)
	}

	return nil
}

func (c *ResolvedUDPConn) attemptDialNewAddr(newAddr *net.UDPAddr) error {
	connUDP, err := c.dialFunc(newAddr.Network(), nil, newAddr)
	if err != nil {
		// failed to dial new address
		return err
	}

	c.connMtx.Lock()
	c.destAddr = newAddr
	// store prev to close later
	prevConn := c.conn
	c.conn = connUDP
	c.connMtx.Unlock()

	if prevConn != nil {
		return prevConn.Close()
	}

	return nil
}

// Write calls net.udpConn.Write, if it fails an attempt is made to connect to a new addr, if that succeeds the write is retried before returning
func (c *ResolvedUDPConn) Write(b []byte) (int, error) {
	c.connMtx.RLock()
	bytesWritten, err := c.conn.Write(b)
	c.connMtx.RUnlock()

	if err == nil {
		return bytesWritten, nil
	}

	// attempt to resolve and dial new address in case that's the problem, if resolve and dial succeeds, try write again
	if reconnErr := c.attemptResolveAndDial(); reconnErr == nil {
		c.connMtx.RLock()
		defer c.connMtx.RUnlock()
		return c.conn.Write(b)
	}

	// return original error if reconn fails
	return bytesWritten, err
}

// Close stops the resolveLoop with a deadline of 2 seconds before continuing to close the connection via net.udpConn 's implementation
func (c *ResolvedUDPConn) Close() error {
	c.closeChan <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	select {
	case <-c.doneChan:
	case <-ctx.Done():
		c.logger.Error("resolve loop didnt finish within 2 second timeout, closing conn")
	}
	cancel()

	return c.conn.Close()
}

// SetWriteBuffer defers to the net.udpConn SetWriteBuffer implementation wrapped with a RLock
func (c *ResolvedUDPConn) SetWriteBuffer(bytes int) error {
	c.connMtx.RLock()
	defer c.connMtx.RUnlock()
	return c.conn.SetWriteBuffer(bytes)
}
