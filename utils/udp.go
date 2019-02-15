// Copyright (c) 2017 Uber Technologies, Inc.
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

import "net"

// GetUDPConnection returns an instance of net.UDPConn from the `hostPort` and
// sets its write buffer to the value specified in `maxPacketSize.`
func GetUDPConnection(hostPort string, maxPacketSize int) (*net.UDPConn, error) {
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
	return connUDP, nil
}
