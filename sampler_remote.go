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

package jaeger

import (
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uber/jaeger-client-go/thrift-gen/sampling"
	"github.com/uber/jaeger-client-go/utils"
)

const (
	defaultSamplingRefreshInterval = time.Minute
)

// RemotelyControlledSampler is a delegating sampler that polls a remote server
// for the appropriate sampling strategy, constructs a corresponding sampler and
// delegates to it for sampling decisions.
type RemotelyControlledSampler struct {
	// These fields must be first in the struct because `sync/atomic` expects 64-bit alignment.
	// Cf. https://github.com/uber/jaeger-client-go/issues/155, https://goo.gl/zW7dgq
	closed int64 // 0 - not closed, 1 - closed

	sync.RWMutex
	samplerOptions

	serviceName string
	manager     sampling.SamplingManager
	doneChan    chan *sync.WaitGroup
}

// NewRemotelyControlledSampler creates a sampler that periodically pulls
// the sampling strategy from an HTTP sampling server (e.g. jaeger-agent).
func NewRemotelyControlledSampler(
	serviceName string,
	opts ...SamplerOption,
) *RemotelyControlledSampler {
	options := new(samplerOptions).applyOptionsAndDefaults(opts...)
	sampler := &RemotelyControlledSampler{
		samplerOptions: *options,
		serviceName:    serviceName,
		manager:        &httpSamplingManager{serverURL: options.samplingServerURL},
		doneChan:       make(chan *sync.WaitGroup),
	}
	go sampler.pollController()
	return sampler
}

// IsSampled implements IsSampled() of Sampler.
// TODO (breaking change) remove when Sampler V1 is removed
func (s *RemotelyControlledSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
	return false, nil
}

func (s *RemotelyControlledSampler) OnCreateSpan(span *Span) SamplingDecision {
	return s.sampler.OnCreateSpan(span)
}

func (s *RemotelyControlledSampler) OnSetOperationName(span *Span, operationName string) SamplingDecision {
	return s.sampler.OnSetOperationName(span, operationName)
}

func (s *RemotelyControlledSampler) OnSetTag(span *Span, key string, value interface{}) SamplingDecision {
	return s.sampler.OnSetTag(span, key, value)
}

func (s *RemotelyControlledSampler) OnFinishSpan(span *Span) SamplingDecision {
	return s.sampler.OnFinishSpan(span)
}

// Close implements Close() of Sampler.
func (s *RemotelyControlledSampler) Close() {
	if swapped := atomic.CompareAndSwapInt64(&s.closed, 0, 1); !swapped {
		s.logger.Error("Repeated attempt to close the sampler is ignored")
		return
	}

	var wg sync.WaitGroup
	wg.Add(1)
	s.doneChan <- &wg
	wg.Wait()
}

// Equal implements Equal() of Sampler.
func (s *RemotelyControlledSampler) Equal(other Sampler) bool {
	// NB The Equal() function is expensive and will be removed. See adaptiveSampler.Equal() for
	// more information.
	if o, ok := other.(*RemotelyControlledSampler); ok {
		s.RLock()
		o.RLock()
		defer s.RUnlock()
		defer o.RUnlock()
		return s.sampler.(Sampler).Equal(o.sampler.(Sampler))
	}
	return false
}

func (s *RemotelyControlledSampler) pollController() {
	ticker := time.NewTicker(s.samplingRefreshInterval)
	defer ticker.Stop()
	s.pollControllerWithTicker(ticker)
}

func (s *RemotelyControlledSampler) pollControllerWithTicker(ticker *time.Ticker) {
	for {
		select {
		case <-ticker.C:
			s.updateSampler()
		case wg := <-s.doneChan:
			wg.Done()
			return
		}
	}
}

func (s *RemotelyControlledSampler) getSampler() Sampler {
	s.Lock()
	defer s.Unlock()
	return s.sampler.(Sampler)
}

func (s *RemotelyControlledSampler) setSampler(sampler Sampler) {
	s.Lock()
	defer s.Unlock()
	s.sampler = samplerV1toV2(sampler)
}

func (s *RemotelyControlledSampler) updateSampler() {
	res, err := s.manager.GetSamplingStrategy(s.serviceName)
	if err != nil {
		s.metrics.SamplerQueryFailure.Inc(1)
		s.logger.Infof("Unable to query sampling strategy: %v", err)
		return
	}
	s.Lock()
	defer s.Unlock()

	s.metrics.SamplerRetrieved.Inc(1)
	if strategies := res.GetOperationSampling(); strategies != nil {
		s.updateAdaptiveSampler(strategies)
	} else {
		err = s.updateRateLimitingOrProbabilisticSampler(res)
	}
	if err != nil {
		s.metrics.SamplerUpdateFailure.Inc(1)
		s.logger.Infof("Unable to handle sampling strategy response %+v. Got error: %v", res, err)
		return
	}
	s.metrics.SamplerUpdated.Inc(1)
}

// NB: this function should only be called while holding a Write lock
func (s *RemotelyControlledSampler) updateAdaptiveSampler(strategies *sampling.PerOperationSamplingStrategies) {
	if adaptiveSampler, ok := s.sampler.(*AdaptiveSampler); ok {
		adaptiveSampler.update(strategies)
	} else {
		s.sampler = newAdaptiveSampler(strategies, s.maxOperations)
	}
}

// NB: this function should only be called while holding a Write lock
func (s *RemotelyControlledSampler) updateRateLimitingOrProbabilisticSampler(res *sampling.SamplingStrategyResponse) error {
	var newSampler SamplerV2
	if probabilistic := res.GetProbabilisticSampling(); probabilistic != nil {
		newSampler = newProbabilisticSampler(probabilistic.SamplingRate)
	} else if rateLimiting := res.GetRateLimitingSampling(); rateLimiting != nil {
		newSampler = NewRateLimitingSampler(float64(rateLimiting.MaxTracesPerSecond))
	} else {
		return fmt.Errorf("Unsupported sampling strategy type %v", res.GetStrategyType())
	}
	if !s.sampler.(Sampler).Equal(newSampler.(Sampler)) {
		s.sampler = newSampler
	}
	return nil
}

type httpSamplingManager struct {
	serverURL string
}

func (s *httpSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	var out sampling.SamplingStrategyResponse
	v := url.Values{}
	v.Set("service", serviceName)
	if err := utils.GetJSON(s.serverURL+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
