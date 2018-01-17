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

package jaeger

import (
	"github.com/uber/jaeger-client-go/internal/throttler"
)

// N.B. this class is private as it's a proof of concept of how we would throttle normal
// sampler traffic.

// throttledSampler wraps a sampler and throttles it.
type throttledSampler struct {
	throttler throttler.Throttler
	sampler   Sampler
}

// newThrottledSampler returns a sampler that is throttled.
func newThrottledSampler(sampler Sampler, throttler throttler.Throttler) *throttledSampler {
	return &throttledSampler{
		sampler:   sampler,
		throttler: throttler,
	}
}

func (s *throttledSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
	sampled, tags := s.sampler.IsSampled(id, operation)
	if sampled && !s.throttler.IsThrottled(operation) {
		return sampled, tags
	}
	return false, nil
}

func (s *throttledSampler) Close() {
	s.throttler.Close()
}

func (s *throttledSampler) Equal(other Sampler) bool {
	// Not implemented
	return false
}
