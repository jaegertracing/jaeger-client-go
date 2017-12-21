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

package throttler

import "io"

// Throttler is used to rate limits operations. For example, given how debug spans
// are always sampled, a throttler can be enabled per client to rate limit the amount
// of debug spans a client can start.
type Throttler interface {
	io.Closer

	// IsThrottled determines whether the operation should be throttled.
	IsThrottled(operation string) bool

	// SetUUID sets the UUID that identifies this client instance.
	SetUUID(uuid string)
}

// DefaultThrottler doesn't throttle at all.
type DefaultThrottler struct{}

// IsThrottled implements Throttler#IsThrottled.
func (t DefaultThrottler) IsThrottled(operation string) bool {
	return false
}

// SetUUID implements Throttler#SetUUID.
func (t DefaultThrottler) SetUUID(uuid string) {}

// Close implements Throttler#Close.
func (t DefaultThrottler) Close() error {
	return nil
}
