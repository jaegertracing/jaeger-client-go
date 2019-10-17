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
	"math"
	"sync"

	"github.com/uber/jaeger-client-go/thrift-gen/sampling"
	"github.com/uber/jaeger-client-go/utils"
)

const (
	defaultMaxOperations = 2000
)

// Sampler decides whether a new trace should be sampled or not.
type Sampler interface {
	// IsSampled decides whether a trace with given `id` and `operation`
	// should be sampled. This function will also return the tags that
	// can be used to identify the type of sampling that was applied to
	// the root span. Most simple samplers would return two tags,
	// sampler.type and sampler.param, similar to those used in the Configuration
	IsSampled(id TraceID, operation string) (sampled bool, tags []Tag)

	// Close does a clean shutdown of the sampler, stopping any background
	// go-routines it may have started.
	Close()

	// Equal checks if the `other` sampler is functionally equivalent
	// to this sampler.
	// TODO remove this function. This function is used to determine if 2 samplers are equivalent
	// which does not bode well with the adaptive sampler which has to create all the composite samplers
	// for the comparison to occur. This is expensive to do if only one sampler has changed.
	Equal(other Sampler) bool
}

// -----------------------

// ConstSampler is a sampler that always makes the same decision.
type ConstSampler struct {
	legacySamplerV1Base
	Decision bool
	tags     []Tag
}

// NewConstSampler creates a ConstSampler.
func NewConstSampler(sample bool) *ConstSampler {
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeConst},
		{key: SamplerParamTagKey, value: sample},
	}
	s := &ConstSampler{
		Decision: sample,
		tags:     tags,
	}
	s.delegate = s.IsSampled
	return s
}

// IsSampled implements IsSampled() of Sampler.
func (s *ConstSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
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

// String is used to log sampler details.
func (s *ConstSampler) String() string {
	return fmt.Sprintf("ConstSampler(decision=%t)", s.Decision)
}

// -----------------------

// ProbabilisticSampler is a sampler that randomly samples a certain percentage
// of traces.
type ProbabilisticSampler struct {
	legacySamplerV1Base
	samplingRate     float64
	samplingBoundary uint64
	tags             []Tag
}

const maxRandomNumber = ^(uint64(1) << 63) // i.e. 0x7fffffffffffffff

// NewProbabilisticSampler creates a sampler that randomly samples a certain percentage of traces specified by the
// samplingRate, in the range between 0.0 and 1.0.
//
// It relies on the fact that new trace IDs are 63bit random numbers themselves, thus making the sampling decision
// without generating a new random number, but simply calculating if traceID < (samplingRate * 2^63).
// TODO remove the error from this function for next major release
func NewProbabilisticSampler(samplingRate float64) (*ProbabilisticSampler, error) {
	if samplingRate < 0.0 || samplingRate > 1.0 {
		return nil, fmt.Errorf("Sampling Rate must be between 0.0 and 1.0, received %f", samplingRate)
	}
	return newProbabilisticSampler(samplingRate), nil
}

func newProbabilisticSampler(samplingRate float64) *ProbabilisticSampler {
	samplingRate = math.Max(0.0, math.Min(samplingRate, 1.0))
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeProbabilistic},
		{key: SamplerParamTagKey, value: samplingRate},
	}
	s := &ProbabilisticSampler{
		samplingRate:     samplingRate,
		samplingBoundary: uint64(float64(maxRandomNumber) * samplingRate),
		tags:             tags,
	}
	s.delegate = s.IsSampled
	return s
}

// SamplingRate returns the sampling probability this sampled was constructed with.
func (s *ProbabilisticSampler) SamplingRate() float64 {
	return s.samplingRate
}

// IsSampled implements IsSampled() of Sampler.
func (s *ProbabilisticSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
	return s.samplingBoundary >= id.Low, s.tags
}

// Close implements Close() of Sampler.
func (s *ProbabilisticSampler) Close() {
	// nothing to do
}

// Equal implements Equal() of Sampler.
func (s *ProbabilisticSampler) Equal(other Sampler) bool {
	if o, ok := other.(*ProbabilisticSampler); ok {
		return s.samplingBoundary == o.samplingBoundary
	}
	return false
}

// String is used to log sampler details.
func (s *ProbabilisticSampler) String() string {
	return fmt.Sprintf("ProbabilisticSampler(samplingRate=%v)", s.samplingRate)
}

// -----------------------

type RateLimitingSampler struct {
	legacySamplerV1Base
	maxTracesPerSecond float64
	rateLimiter        utils.RateLimiter
	tags               []Tag
}

// NewRateLimitingSampler creates a sampler that samples at most maxTracesPerSecond. The distribution of sampled
// traces follows burstiness of the service, i.e. a service with uniformly distributed requests will have those
// requests sampled uniformly as well, but if requests are bursty, especially sub-second, then a number of
// sequential requests can be sampled each second.
func NewRateLimitingSampler(maxTracesPerSecond float64) *RateLimitingSampler {
	tags := []Tag{
		{key: SamplerTypeTagKey, value: SamplerTypeRateLimiting},
		{key: SamplerParamTagKey, value: maxTracesPerSecond},
	}
	s := &RateLimitingSampler{
		maxTracesPerSecond: maxTracesPerSecond,
		rateLimiter:        utils.NewRateLimiter(maxTracesPerSecond, math.Max(maxTracesPerSecond, 1.0)),
		tags:               tags,
	}
	s.delegate = s.IsSampled
	return s
}

// IsSampled implements IsSampled() of Sampler.
func (s *RateLimitingSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
	return s.rateLimiter.CheckCredit(1.0), s.tags
}

func (s *RateLimitingSampler) Close() {
	// nothing to do
}

func (s *RateLimitingSampler) Equal(other Sampler) bool {
	if o, ok := other.(*RateLimitingSampler); ok {
		return s.maxTracesPerSecond == o.maxTracesPerSecond
	}
	return false
}

// String is used to log sampler details.
func (s *RateLimitingSampler) String() string {
	return fmt.Sprintf("RateLimitingSampler(maxTracesPerSecond=%v)", s.maxTracesPerSecond)
}

// -----------------------

// GuaranteedThroughputProbabilisticSampler is a sampler that leverages both ProbabilisticSampler and
// RateLimitingSampler. The RateLimitingSampler is used as a guaranteed lower bound sampler such that
// every operation is sampled at least once in a time interval defined by the lowerBound. ie a lowerBound
// of 1.0 / (60 * 10) will sample an operation at least once every 10 minutes.
//
// The ProbabilisticSampler is given higher priority when tags are emitted, ie. if IsSampled() for both
// samplers return true, the tags for ProbabilisticSampler will be used.
type GuaranteedThroughputProbabilisticSampler struct {
	probabilisticSampler *ProbabilisticSampler
	lowerBoundSampler    Sampler
	tags                 []Tag
	samplingRate         float64
	lowerBound           float64
}

// NewGuaranteedThroughputProbabilisticSampler returns a delegating sampler that applies both
// ProbabilisticSampler and RateLimitingSampler.
func NewGuaranteedThroughputProbabilisticSampler(
	lowerBound, samplingRate float64,
) (*GuaranteedThroughputProbabilisticSampler, error) {
	return newGuaranteedThroughputProbabilisticSampler(lowerBound, samplingRate), nil
}

func newGuaranteedThroughputProbabilisticSampler(lowerBound, samplingRate float64) *GuaranteedThroughputProbabilisticSampler {
	s := &GuaranteedThroughputProbabilisticSampler{
		lowerBoundSampler: NewRateLimitingSampler(lowerBound),
		lowerBound:        lowerBound,
	}
	s.setProbabilisticSampler(samplingRate)
	return s
}

func (s *GuaranteedThroughputProbabilisticSampler) setProbabilisticSampler(samplingRate float64) {
	if s.probabilisticSampler == nil || s.samplingRate != samplingRate {
		s.probabilisticSampler = newProbabilisticSampler(samplingRate)
		s.samplingRate = s.probabilisticSampler.SamplingRate()
		s.tags = []Tag{
			{key: SamplerTypeTagKey, value: SamplerTypeLowerBound},
			{key: SamplerParamTagKey, value: s.samplingRate},
		}
	}
}

// IsSampled implements IsSampled() of Sampler.
func (s *GuaranteedThroughputProbabilisticSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
	if sampled, tags := s.probabilisticSampler.IsSampled(id, operation); sampled {
		s.lowerBoundSampler.IsSampled(id, operation)
		return true, tags
	}
	sampled, _ := s.lowerBoundSampler.IsSampled(id, operation)
	return sampled, s.tags
}

// Close implements Close() of Sampler.
func (s *GuaranteedThroughputProbabilisticSampler) Close() {
	s.probabilisticSampler.Close()
	s.lowerBoundSampler.Close()
}

// Equal implements Equal() of Sampler.
func (s *GuaranteedThroughputProbabilisticSampler) Equal(other Sampler) bool {
	// NB The Equal() function is expensive and will be removed. See AdaptiveSampler.Equal() for
	// more information.
	return false
}

// this function should only be called while holding a Write lock
func (s *GuaranteedThroughputProbabilisticSampler) update(lowerBound, samplingRate float64) {
	s.setProbabilisticSampler(samplingRate)
	if s.lowerBound != lowerBound {
		s.lowerBoundSampler = NewRateLimitingSampler(lowerBound)
		s.lowerBound = lowerBound
	}
}

// -----------------------

type AdaptiveSampler struct {
	sync.RWMutex

	samplers       map[string]*GuaranteedThroughputProbabilisticSampler
	defaultSampler *ProbabilisticSampler
	lowerBound     float64
	maxOperations  int
}

// NewAdaptiveSampler returns a delegating sampler that applies both ProbabilisticSampler and
// RateLimitingSampler via the GuaranteedThroughputProbabilisticSampler. This sampler keeps track of all
// operations and delegates calls to the respective GuaranteedThroughputProbabilisticSampler.
// TODO (breaking change) remove error from return value
func NewAdaptiveSampler(strategies *sampling.PerOperationSamplingStrategies, maxOperations int) (*AdaptiveSampler, error) {
	return newAdaptiveSampler(strategies, maxOperations), nil
}

func newAdaptiveSampler(strategies *sampling.PerOperationSamplingStrategies, maxOperations int) *AdaptiveSampler {
	samplers := make(map[string]*GuaranteedThroughputProbabilisticSampler)
	for _, strategy := range strategies.PerOperationStrategies {
		sampler := newGuaranteedThroughputProbabilisticSampler(
			strategies.DefaultLowerBoundTracesPerSecond,
			strategy.ProbabilisticSampling.SamplingRate,
		)
		samplers[strategy.Operation] = sampler
	}
	return &AdaptiveSampler{
		samplers:       samplers,
		defaultSampler: newProbabilisticSampler(strategies.DefaultSamplingProbability),
		lowerBound:     strategies.DefaultLowerBoundTracesPerSecond,
		maxOperations:  maxOperations,
	}
}

func (s *AdaptiveSampler) IsSampled(id TraceID, operation string) (bool, []Tag) {
	return false, nil
}

func (s *AdaptiveSampler) OnCreateSpan(span *Span) SamplingDecision {
	operationName := span.OperationName()
	samplerV1 := s.getSamplerForOperation(operationName)
	sampled, tags := samplerV1.IsSampled(span.context.TraceID(), operationName)
	return SamplingDecision{sample: sampled, retryable: true, tags: tags}
}

func (s *AdaptiveSampler) OnSetOperationName(span *Span, operationName string) SamplingDecision {
	samplerV1 := s.getSamplerForOperation(operationName)
	sampled, tags := samplerV1.IsSampled(span.context.TraceID(), operationName)
	return SamplingDecision{sample: sampled, retryable: false, tags: tags}
}

func (s *AdaptiveSampler) OnSetTag(span *Span, key string, value interface{}) SamplingDecision {
	return SamplingDecision{sample: false, retryable: true}
}

func (s *AdaptiveSampler) OnFinishSpan(span *Span) SamplingDecision {
	return SamplingDecision{sample: false, retryable: true}
}

func (s *AdaptiveSampler) getSamplerForOperation(operation string) Sampler {
	s.RLock()
	sampler, ok := s.samplers[operation]
	if ok {
		defer s.RUnlock()
		return sampler
	}
	s.RUnlock()
	s.Lock()
	defer s.Unlock()

	// Check if sampler has already been created
	sampler, ok = s.samplers[operation]
	if ok {
		return sampler
	}
	// Store only up to maxOperations of unique ops.
	if len(s.samplers) >= s.maxOperations {
		return s.defaultSampler
	}
	newSampler := newGuaranteedThroughputProbabilisticSampler(s.lowerBound, s.defaultSampler.SamplingRate())
	s.samplers[operation] = newSampler
	return newSampler
}

func (s *AdaptiveSampler) Close() {
	s.Lock()
	defer s.Unlock()
	for _, sampler := range s.samplers {
		sampler.Close()
	}
	s.defaultSampler.Close()
}

// TODO (breaking change) remove this in the future
func (s *AdaptiveSampler) Equal(other Sampler) bool {
	// NB The Equal() function is overly expensive for AdaptiveSampler since it's composed of multiple
	// samplers which all need to be initialized before this function can be called for a comparison.
	// Therefore, AdaptiveSampler uses the update() function to only alter the samplers that need
	// changing. Hence this function always returns false so that the update function can be called.
	// Once the Equal() function is removed from the Sampler API, this will no longer be needed.
	return false
}

func (s *AdaptiveSampler) update(strategies *sampling.PerOperationSamplingStrategies) {
	s.Lock()
	defer s.Unlock()
	newSamplers := map[string]*GuaranteedThroughputProbabilisticSampler{}
	for _, strategy := range strategies.PerOperationStrategies {
		operation := strategy.Operation
		samplingRate := strategy.ProbabilisticSampling.SamplingRate
		lowerBound := strategies.DefaultLowerBoundTracesPerSecond
		if sampler, ok := s.samplers[operation]; ok {
			sampler.update(lowerBound, samplingRate)
			newSamplers[operation] = sampler
		} else {
			sampler := newGuaranteedThroughputProbabilisticSampler(
				lowerBound,
				samplingRate,
			)
			newSamplers[operation] = sampler
		}
	}
	s.lowerBound = strategies.DefaultLowerBoundTracesPerSecond
	if s.defaultSampler.SamplingRate() != strategies.DefaultSamplingProbability {
		s.defaultSampler = newProbabilisticSampler(strategies.DefaultSamplingProbability)
	}
	s.samplers = newSamplers
}
