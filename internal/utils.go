// Copyright (c) 2018 The Jaeger Authors.
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

package internal

import (
	"encoding/binary"
	"net"
)

// UnpackUint32AsIP does the reverse of utils.PackIPAsUint32
func UnpackUint32AsIP(ip uint32) net.IP {
	localIP := make(net.IP, 4)
	binary.BigEndian.PutUint32(localIP, ip)
	return localIP
}
