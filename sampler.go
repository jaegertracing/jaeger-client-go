// Copyright (c) 2016 Uber Technologies, Inc.
//
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

package jaeger

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/uber/jaeger-client-go/thrift-gen/sampling"
	"github.com/uber/jaeger-client-go/utils"
)

const defaultSamplingServerHostPort = "localhost:5778"

// Sampler decides whether a new trace should be sampled or not.
type Sampler interface {
	// IsSampled decides whether a trace with given `id` and `operation`
	// should be sampled. This function will also return the tags that
	// can be used to identify the type of sampling that was applied to
	// the root span. Most simple samplers would return two tags,
	// sampler.type and sampler.param, similar to those used in the Configuration
	IsSampled(id uint64, operation string) (sampled bool, tags []Tag)

	// Close does a clean shutdown of the sampler, stopping any background
	// go-routines it may have started.
	Close()

	// Equal checks if the `other` sampler is functionally equivalent
	// to this sampler.
	Equal(other Sampler) bool
}

// -----------------------

// ConstSampler is a sampler that always makes the same decision.
type ConstSampler struct {
	Decision bool
	tags     []Tag
}

// NewConstSampler creates a ConstSampler.
func NewConstSampler(sample bool) Sampler {
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeConst},
		{key: SamplerParamTagKey, value: sample},
	}
	return &ConstSampler{Decision: sample, tags: tags}
}

// IsSampled implements IsSampled() of Sampler.
func (s *ConstSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	return s.Decision, s.tags
}

// Close implements Close() of Sampler.
func (s *ConstSampler) Close() {
	// nothing to do
}

// Equal implements Equal() of Sampler.
func (s *ConstSampler) Equal(other Sampler) bool {
	if o, ok := other.(*ConstSampler); ok {
		return s.Decision == o.Decision
	}
	return false
}

// -----------------------

// ProbabilisticSampler is a sampler that randomly samples a certain percentage
// of traces.
type ProbabilisticSampler struct {
	SamplingBoundary uint64
	tags             []Tag
}

const maxRandomNumber = ^(uint64(1) << 63) // i.e. 0x7fffffffffffffff

// NewProbabilisticSampler creates a sampler that randomly samples a certain percentage of traces specified by the
// samplingRate, in the range between 0.0 and 1.0.
//
// It relies on the fact that new trace IDs are 63bit random numbers themselves, thus making the sampling decision
// without generating a new random number, but simply calculating if traceID < (samplingRate * 2^63).
func NewProbabilisticSampler(samplingRate float64) (Sampler, error) {
	if samplingRate < 0.0 || samplingRate > 1.0 {
		return nil, fmt.Errorf("Sampling Rate must be between 0.0 and 1.0, received %f", samplingRate)
	}
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeProbabilistic},
		{key: SamplerParamTagKey, value: samplingRate},
	}
	return &ProbabilisticSampler{
		SamplingBoundary: uint64(float64(maxRandomNumber) * samplingRate),
		tags:             tags}, nil
}

// IsSampled implements IsSampled() of Sampler.
func (s *ProbabilisticSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	return s.SamplingBoundary >= id, s.tags
}

// Close implements Close() of Sampler.
func (s *ProbabilisticSampler) Close() {
	// nothing to do
}

// Equal implements Equal() of Sampler.
func (s *ProbabilisticSampler) Equal(other Sampler) bool {
	if o, ok := other.(*ProbabilisticSampler); ok {
		return s.SamplingBoundary == o.SamplingBoundary
	}
	return false
}

// -----------------------

type rateLimitingSampler struct {
	maxTracesPerSecond float64
	rateLimiter        utils.RateLimiter
	tags               []Tag
}

// NewRateLimitingSampler creates a sampler that samples at most maxTracesPerSecond. The distribution of sampled
// traces follows burstiness of the service, i.e. a service with uniformly distributed requests will have those
// requests sampled uniformly as well, but if requests are bursty, especially sub-second, then a number of
// sequential requests can be sampled each second.
func NewRateLimitingSampler(maxTracesPerSecond float64) (Sampler, error) {
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeRateLimiting},
		{key: SamplerParamTagKey, value: maxTracesPerSecond},
	}
	return &rateLimitingSampler{
		maxTracesPerSecond: maxTracesPerSecond,
		rateLimiter:        utils.NewRateLimiter(maxTracesPerSecond),
		tags:               tags,
	}, nil
}

// IsSampled implements IsSampled() of Sampler.
func (s *rateLimitingSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	return s.rateLimiter.CheckCredit(1.0), s.tags
}

func (s *rateLimitingSampler) Close() {
	// nothing to do
}

func (s *rateLimitingSampler) Equal(other Sampler) bool {
	if o, ok := other.(*rateLimitingSampler); ok {
		return s.maxTracesPerSecond == o.maxTracesPerSecond
	}
	return false
}

// -----------------------

type adaptiveSampler struct {
	rateLimitingSamplers  map[string]Sampler
	probabilisticSamplers map[string]Sampler

	// TODO expand to keep track of newly seen operations
	// TODO add a cap on the number of operations
	// TODO need default rate limit and probabilistic rates for first time operations
}

type adaptiveSamplingRates struct {
	samplingRate       *float64
	maxTracesPerSecond *float64
}

// NewAdaptiveSampler adaptiveSampler is a delegating sampler that applies both probabilisticSampler and
// rateLimitingSampler per operation. The probabilisticSampler is given higher priority when tags are emitted,
// ie. if IsSampled() for both samplers return true, the tags for probabilisticSampler will be used.
//
// This sampler assumes that it will be used by RemotelyControlledSampler hence it is up to the user to
// ensure this is thread-safe if not used by RemotelyControlledSampler.
func NewAdaptiveSampler(samplingRates map[string]*adaptiveSamplingRates) (Sampler, error) {
	probabilisticSamplers := make(map[string]Sampler)
	rateLimitingSamplers := make(map[string]Sampler)
	for operation, rates := range samplingRates {
		if rates.samplingRate != nil {
			sampler, err := NewProbabilisticSampler(*rates.samplingRate)
			if err != nil {
				return nil, err
			}
			probabilisticSamplers[operation] = sampler
		}
		if rates.maxTracesPerSecond != nil {
			sampler, err := NewRateLimitingSampler(*rates.maxTracesPerSecond)
			if err != nil {
				return nil, err
			}
			rateLimitingSamplers[operation] = sampler
		}
	}
	return &adaptiveSampler{
		probabilisticSamplers: probabilisticSamplers,
		rateLimitingSamplers:  rateLimitingSamplers,
	}, nil
}

func (s *adaptiveSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	probabilisticSampler := s.probabilisticSamplers[operation]
	if probabilisticSampler != nil {
		if sampled, tags := probabilisticSampler.IsSampled(id, operation); sampled {
			return true, tags
		}
	}
	rateLimitingSampler := s.rateLimitingSamplers[operation]
	if rateLimitingSampler != nil {
		return rateLimitingSampler.IsSampled(id, operation)
	}
	// It should not get here, ideally every adaptiveSampler for an operation should have at least one sampler
	return false, nil
}

func (s *adaptiveSampler) Close() {
	for _, probabilisticSampler := range s.probabilisticSamplers {
		probabilisticSampler.Close()
	}
	for _, rateLimitingSampler := range s.rateLimitingSamplers {
		rateLimitingSampler.Close()
	}
}

func (s *adaptiveSampler) Equal(other Sampler) bool {
	if o, ok := other.(*adaptiveSampler); ok {
		if !mapKeysEqual(s.probabilisticSamplers, o.probabilisticSamplers) {
			return false
		}
		for operation, sampler := range s.probabilisticSamplers {
			if !sampler.Equal(o.probabilisticSamplers[operation]) {
				return false
			}
		}
		for operation, sampler := range s.rateLimitingSamplers {
			if !sampler.Equal(o.rateLimitingSamplers[operation]) {
				return false
			}
		}
		return true
	}
	return false
}

func mapKeysEqual(m1, m2 map[string]Sampler) bool {
	if len(m1) != len(m2) {
		return false
	}
	for k := range m1 {
		if _, ok := m2[k]; !ok {
			return false
		}
	}
	return true
}

// -----------------------

// RemotelyControlledSampler is a delegating sampler that polls a remote server
// for the appropriate sampling strategy, constructs a corresponding sampler and
// delegates to it for sampling decisions.
type RemotelyControlledSampler struct {
	sync.RWMutex

	serviceName string
	manager     sampling.SamplingManager
	logger      Logger
	timer       *time.Ticker
	sampler     Sampler
	pollStopped sync.WaitGroup
	metrics     Metrics
}

type httpSamplingManager struct {
	serverURL string
}

func (s *httpSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	var out sampling.SamplingStrategyResponse
	v := url.Values{}
	v.Set("service", serviceName)
	if err := utils.GetJSON(s.serverURL+"/?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// NewRemotelyControlledSampler creates a sampler that periodically pulls
// the sampling strategy from an HTTP sampling server (e.g. jaeger-agent).
func NewRemotelyControlledSampler(
	serviceName string,
	initial Sampler,
	hostPort string,
	metrics *Metrics,
	logger Logger,
) *RemotelyControlledSampler {
	if hostPort == "" {
		hostPort = defaultSamplingServerHostPort
	}
	manager := &httpSamplingManager{serverURL: "http://" + hostPort}
	if logger == nil {
		logger = NullLogger
	}
	sampler := &RemotelyControlledSampler{
		serviceName: serviceName,
		manager:     manager,
		logger:      logger,
		metrics:     *metrics,
		timer:       time.NewTicker(1 * time.Minute),
	}
	if initial != nil {
		sampler.sampler = initial
	} else {
		sampler.sampler, _ = NewProbabilisticSampler(0.001)
	}
	go sampler.pollController()
	return sampler
}

// IsSampled implements IsSampled() of Sampler.
func (s *RemotelyControlledSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	s.RLock()
	defer s.RUnlock()
	return s.sampler.IsSampled(id, operation)
}

// Close implements Close() of Sampler.
func (s *RemotelyControlledSampler) Close() {
	s.RLock()
	s.timer.Stop()
	s.RUnlock()

	s.pollStopped.Wait()
}

// Equal implements Equal() of Sampler.
func (s *RemotelyControlledSampler) Equal(other Sampler) bool {
	if o, ok := other.(*RemotelyControlledSampler); ok {
		s.RLock()
		o.RLock()
		defer s.RUnlock()
		defer o.RUnlock()
		return s.sampler.Equal(o.sampler)
	}
	return false
}

func (s *RemotelyControlledSampler) pollController() {
	// in unit tests we re-assign the timer Ticker, so need to lock to avoid data races
	s.Lock()
	timer := s.timer
	s.Unlock()

	for range timer.C {
		s.updateSampler()
	}
	s.pollStopped.Add(1)
}

func (s *RemotelyControlledSampler) updateSampler() {
	if res, err := s.manager.GetSamplingStrategy(s.serviceName); err == nil {
		if sampler, err := s.extractSampler(res); err == nil {
			s.metrics.SamplerRetrieved.Inc(1)
			if !s.sampler.Equal(sampler) {
				s.Lock()
				s.sampler = sampler
				s.Unlock()
				s.metrics.SamplerUpdated.Inc(1)
			}
		} else {
			s.metrics.SamplerParsingFailure.Inc(1)
			s.logger.Infof("Unable to handle sampling strategy response %+v. Got error: %v", res, err)
		}
	} else {
		s.metrics.SamplerQueryFailure.Inc(1)
	}
}

func (s *RemotelyControlledSampler) extractSampler(res *sampling.SamplingStrategyResponse) (Sampler, error) {
	// TODO create adaptive sampler
	if probabilistic := res.GetProbabilisticSampling(); probabilistic != nil {
		sampler, err := NewProbabilisticSampler(probabilistic.SamplingRate)
		if err != nil {
			return nil, err
		}
		return sampler, nil
	}
	if rateLimiting := res.GetRateLimitingSampling(); rateLimiting != nil {
		sampler, err := NewRateLimitingSampler(float64(rateLimiting.MaxTracesPerSecond))
		if err != nil {
			return nil, err
		}
		return sampler, nil
	}
	return nil, fmt.Errorf("Unsupported sampling strategy type %v", res.GetStrategyType())
}
