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

var (
	defaultSamplingProbability = 0.001

	// The default lower bound rate limit is 1 per 10 minutes
	defaultMaxTracesPerSecond = 1.0 / (60 * 10)
)

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

type lowerBoundSampler struct {
	rateLimitingSampler Sampler
	samplingRate        float64
	tags                []Tag
}

// NewLowerBoundSampler creates a sampler that samples at most maxTracesPerSecond. It is a wrapper around
// a rateLimitingSampler with different tags since it's intended to be used by the adaptiveSampler.
// The sampler.param tag is sampling rate rather than maxTracesPerSecond. This is used to tell the collector
// that the trace sampled by this sampler used the samplingRate to determine the throughput for the operation.
func NewLowerBoundSampler(maxTracesPerSecond, samplingRate float64) (Sampler, error) {
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeLowerBound},
		{key: SamplerParamTagKey, value: samplingRate},
	}
	rateLimitingSampler, err := NewRateLimitingSampler(maxTracesPerSecond)
	if err != nil {
		return nil, err
	}
	return &lowerBoundSampler{
		rateLimitingSampler: rateLimitingSampler,
		samplingRate:        samplingRate,
		tags:                tags,
	}, nil
}

// IsSampled implements IsSampled() of Sampler.
func (s *lowerBoundSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	sampled, _ := s.rateLimitingSampler.IsSampled(id, operation)
	return sampled, s.tags
}

func (s *lowerBoundSampler) Close() {
	s.rateLimitingSampler.Close()
}

func (s *lowerBoundSampler) Equal(other Sampler) bool {
	if o, ok := other.(*lowerBoundSampler); ok {
		return s.samplingRate == o.samplingRate &&
			s.rateLimitingSampler.Equal(o.rateLimitingSampler)
	}
	return false
}

// -----------------------

// sampler for the operation level
type operationSampler struct {
	probabilisticSampler Sampler
	lowerBoundSampler    Sampler
	operation            string
}

// NewOperationSampler operationSampler is a delegating sampler that applies both probabilisticSampler and
// rateLimitingSampler. The probabilisticSampler is given higher priority when tags are emitted,
// ie. if IsSampled() for both samplers return true, the tags for probabilisticSampler will be used.
func NewOperationSampler(operation string, maxTracesPerSecond, samplingRate float64) (Sampler, error) {
	probabilisticSampler, err := NewProbabilisticSampler(samplingRate)
	if err != nil {
		return nil, err
	}
	lowerBoundSampler, err := NewLowerBoundSampler(maxTracesPerSecond, samplingRate)
	if err != nil {
		return nil, err
	}
	return &operationSampler{
		probabilisticSampler: probabilisticSampler,
		lowerBoundSampler:    lowerBoundSampler,
		operation:            operation,
	}, nil
}

func (s *operationSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	if sampled, tags := s.probabilisticSampler.IsSampled(id, operation); sampled {
		return true, tags
	}
	return s.lowerBoundSampler.IsSampled(id, operation)
}

func (s *operationSampler) Close() {
	s.probabilisticSampler.Close()
	s.lowerBoundSampler.Close()
}

func (s *operationSampler) Equal(other Sampler) bool {
	if o, ok := other.(*operationSampler); ok {
		return s.operation == o.operation && s.probabilisticSampler.Equal(o.probabilisticSampler) &&
			s.lowerBoundSampler.Equal(o.lowerBoundSampler)
	}
	return false
}

// -----------------------

type adaptiveSampler struct {
	samplers map[string]Sampler

	// TODO expand to keep track of newly seen operations
	// TODO add a cap on the number of operations
	// TODO need default rate limit and probabilistic rates for first time operations
}

// NewAdaptiveSampler adaptiveSampler is a delegating sampler that applies both probabilisticSampler and
// rateLimitingSampler via the operationSampler. This sampler keeps track of all operations and delegates
// calls to the respective operationSampler.
func NewAdaptiveSampler(maxTracesPerSecond float64, samplingRates map[string]float64) (Sampler, error) {
	samplers := make(map[string]Sampler)
	for operation, samplingRate := range samplingRates {
		probabilisticSampler, err := NewProbabilisticSampler(samplingRate)
		if err != nil {
			return nil, err
		}
		lowerBoundSampler, err := NewLowerBoundSampler(maxTracesPerSecond, samplingRate)
		if err != nil {
			return nil, err
		}
		samplers[operation] = &operationSampler{
			probabilisticSampler: probabilisticSampler,
			lowerBoundSampler:    lowerBoundSampler,
			operation:            operation,
		}
	}
	return &adaptiveSampler{
		samplers: samplers,
	}, nil
}

func (s *adaptiveSampler) IsSampled(id uint64, operation string) (bool, []Tag) {
	sampler, ok := s.samplers[operation]
	if !ok {
		// TODO handle first time operation
		return false, nil
	}
	return sampler.IsSampled(id, operation)
}

func (s *adaptiveSampler) Close() {
	for _, sampler := range s.samplers {
		sampler.Close()
	}
}

func (s *adaptiveSampler) Equal(other Sampler) bool {
	if o, ok := other.(*adaptiveSampler); ok {
		if !mapKeysEqual(s.samplers, o.samplers) {
			return false
		}
		for operation, sampler := range s.samplers {
			if !sampler.Equal(o.samplers[operation]) {
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
