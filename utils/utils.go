// Copyright (c) 2015 Uber Technologies, Inc.

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

package utils

import (
	"errors"
	"net"
	"strconv"
	"strings"
)

var (
	// ErrEmptyIP an error for empty ip strings
	ErrEmptyIP = errors.New("empty string given for ip")

	// ErrNotHostColonPort an error for invalid host port string
	ErrNotHostColonPort = errors.New("expecting host:port")

	// ErrNotFourOctets an error for the wrong number of octets after splitting a string
	ErrNotFourOctets = errors.New("Wrong number of octets")
)

func GetLocalIP() net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}

	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP
			}
		}
	}

	return net.IPv4(127, 0, 0, 1)
}

// IPToUint32 converts a string ip to an uint32
func IPToUint32(ip string) (uint32, error) {
	if ip == "" {
		return 0, ErrEmptyIP
	}

	if ip == "localhost" {
		return 127<<24 | 1, nil
	}

	octets := strings.Split(ip, ".")
	if len(octets) != 4 {
		return 0, ErrNotFourOctets
	}

	var intIP uint32
	for i := 0; i < 4; i++ {
		octet, err := strconv.Atoi(octets[i])
		if err != nil {
			return 0, err
		}
		intIP = (intIP << 8) | uint32(octet)
	}

	return intIP, nil
}

// ParsePort converts port number from string to uin16
func ParsePort(portString string) (uint16, error) {
	port, err := strconv.ParseUint(portString, 10, 16)
	return uint16(port), err
}
