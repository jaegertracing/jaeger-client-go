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
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go/thrift-gen/sampling"
	"github.com/uber/jaeger-client-go/utils"
)

const (
	testOperationName          = "op"
	testFirstTimeOperationName = "firstTimeOp"

	testDefaultSamplingProbability = 0.5
	testMaxID                      = uint64(1) << 62
	testDefaultMaxOperations       = 10
)

var (
	testProbabilisticExpectedTags = []Tag{
		{"sampler.type", "probabilistic"},
		{"sampler.param", 0.5},
	}
	testLowerBoundExpectedTags = []Tag{
		{"sampler.type", "lowerbound"},
		{"sampler.param", 0.5},
	}
)

func TestSamplerTags(t *testing.T) {
	prob, err := NewProbabilisticSampler(0.1)
	require.NoError(t, err)
	rate := NewRateLimitingSampler(0.1)
	remote := &RemotelyControlledSampler{}
	remote.sampler = NewConstSampler(true)
	tests := []struct {
		sampler  SamplerV2
		typeTag  string
		paramTag interface{}
	}{
		{NewConstSampler(true), "const", true},
		{NewConstSampler(false), "const", false},
		{prob, "probabilistic", 0.1},
		{rate, "ratelimiting", 0.1},
		{remote, "const", true},
	}
	for _, test := range tests {
		decision := test.sampler.OnCreateSpan(makeSpan(0, testOperationName))
		assert.Equal(t, makeSamplerTags(test.typeTag, test.paramTag), decision.tags)
	}
}

func TestProbabilisticSamplerErrors(t *testing.T) {
	_, err := NewProbabilisticSampler(-0.1)
	assert.Error(t, err)
	_, err = NewProbabilisticSampler(1.1)
	assert.Error(t, err)
}

func TestProbabilisticSampler(t *testing.T) {
	sampler, _ := NewProbabilisticSampler(0.5)
	sampled, tags := sampler.IsSampled(TraceID{Low: testMaxID + 10}, testOperationName)
	assert.False(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
	sampled, tags = sampler.IsSampled(TraceID{Low: testMaxID - 20}, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
	sampler2, _ := NewProbabilisticSampler(0.5)
	assert.True(t, sampler.Equal(sampler2))
	assert.False(t, sampler.Equal(NewConstSampler(true)))
}

func TestProbabilisticSamplerPerformance(t *testing.T) {
	t.Skip("Skipped performance test")
	sampler, _ := NewProbabilisticSampler(0.01)
	rand := utils.NewRand(8736823764)
	var count uint64
	for i := 0; i < 100000000; i++ {
		id := TraceID{Low: uint64(rand.Int63())}
		if sampled, _ := sampler.IsSampled(id, testOperationName); sampled {
			count++
		}
	}
	// println("Sampled:", count, "rate=", float64(count)/float64(100000000))
	// Sampled: 999829 rate= 0.009998290
}

func TestRateLimitingSampler(t *testing.T) {
	sampler := NewRateLimitingSampler(2)
	sampler2 := NewRateLimitingSampler(2)
	sampler3 := NewRateLimitingSampler(3)
	assert.True(t, sampler.Equal(sampler2))
	assert.False(t, sampler.Equal(sampler3))
	assert.False(t, sampler.Equal(NewConstSampler(false)))

	sampler = NewRateLimitingSampler(2)
	sampled, _ := sampler.IsSampled(TraceID{}, testOperationName)
	assert.True(t, sampled)
	sampled, _ = sampler.IsSampled(TraceID{}, testOperationName)
	assert.True(t, sampled)
	sampled, _ = sampler.IsSampled(TraceID{}, testOperationName)
	assert.False(t, sampled)

	sampler = NewRateLimitingSampler(0.1)
	sampled, _ = sampler.IsSampled(TraceID{}, testOperationName)
	assert.True(t, sampled)
	sampled, _ = sampler.IsSampled(TraceID{}, testOperationName)
	assert.False(t, sampled)
}

func TestGuaranteedThroughputProbabilisticSamplerUpdate(t *testing.T) {
	samplingRate := 0.5
	lowerBound := 2.0
	sampler, err := NewGuaranteedThroughputProbabilisticSampler(lowerBound, samplingRate)
	assert.NoError(t, err)

	assert.Equal(t, lowerBound, sampler.lowerBound)
	assert.Equal(t, samplingRate, sampler.samplingRate)

	newSamplingRate := 0.6
	newLowerBound := 1.0
	sampler.update(newLowerBound, newSamplingRate)
	assert.Equal(t, newLowerBound, sampler.lowerBound)
	assert.Equal(t, newSamplingRate, sampler.samplingRate)

	newSamplingRate = 1.1
	sampler.update(newLowerBound, newSamplingRate)
	assert.Equal(t, 1.0, sampler.samplingRate)
}

func TestAdaptiveSampler(t *testing.T) {
	samplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: testDefaultSamplingProbability},
		},
	}
	strategies := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       testDefaultSamplingProbability,
		DefaultLowerBoundTracesPerSecond: 1.0,
		PerOperationStrategies:           samplingRates,
	}

	sampler, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	require.NoError(t, err)
	defer sampler.Close()

	decision := sampler.OnCreateSpan(makeSpan(testMaxID+10, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, testLowerBoundExpectedTags, decision.tags)

	decision = sampler.OnCreateSpan(makeSpan(testMaxID-20, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, testProbabilisticExpectedTags, decision.tags)

	decision = sampler.OnCreateSpan(makeSpan(testMaxID+10, testOperationName))
	assert.False(t, decision.sample)

	// This operation is seen for the first time by the sampler
	decision = sampler.OnCreateSpan(makeSpan(testMaxID, testFirstTimeOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, testProbabilisticExpectedTags, decision.tags)
}

func TestAdaptiveSamplerErrors(t *testing.T) {
	strategies := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       testDefaultSamplingProbability,
		DefaultLowerBoundTracesPerSecond: 2.0,
		PerOperationStrategies: []*sampling.OperationSamplingStrategy{
			{
				Operation:             testOperationName,
				ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: -0.1},
			},
		},
	}

	sampler, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.NoError(t, err)
	assert.Equal(t, 0.0, sampler.samplers[testOperationName].samplingRate)

	strategies.PerOperationStrategies[0].ProbabilisticSampling.SamplingRate = 1.1
	sampler, err = NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.NoError(t, err)
	assert.Equal(t, 1.0, sampler.samplers[testOperationName].samplingRate)
}

func TestAdaptiveSamplerUpdate(t *testing.T) {
	samplingRate := 0.1
	lowerBound := 2.0
	samplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: samplingRate},
		},
	}
	strategies := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       testDefaultSamplingProbability,
		DefaultLowerBoundTracesPerSecond: lowerBound,
		PerOperationStrategies:           samplingRates,
	}

	sampler, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.NoError(t, err)

	assert.Equal(t, lowerBound, sampler.lowerBound)
	assert.Equal(t, testDefaultSamplingProbability, sampler.defaultSampler.SamplingRate())
	assert.Len(t, sampler.samplers, 1)

	// Update the sampler with new sampling rates
	newSamplingRate := 0.2
	newLowerBound := 3.0
	newDefaultSamplingProbability := 0.1
	newSamplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: newSamplingRate},
		},
		{
			Operation:             testFirstTimeOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: newSamplingRate},
		},
	}
	strategies = &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       newDefaultSamplingProbability,
		DefaultLowerBoundTracesPerSecond: newLowerBound,
		PerOperationStrategies:           newSamplingRates,
	}

	sampler.update(strategies)
	assert.Equal(t, newLowerBound, sampler.lowerBound)
	assert.Equal(t, newDefaultSamplingProbability, sampler.defaultSampler.SamplingRate())
	assert.Len(t, sampler.samplers, 2)
}

func TestMaxOperations(t *testing.T) {
	samplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 0.1},
		},
	}
	strategies := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       testDefaultSamplingProbability,
		DefaultLowerBoundTracesPerSecond: 2.0,
		PerOperationStrategies:           samplingRates,
	}

	sampler, err := NewAdaptiveSampler(strategies, 1)
	assert.NoError(t, err)

	decision := sampler.OnCreateSpan(makeSpan(testMaxID-10, testFirstTimeOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, testProbabilisticExpectedTags, decision.tags)
}

func TestAdaptiveSampler_lockRaceCondition(t *testing.T) {
	agent, remoteSampler, _ := initAgent(t)
	defer agent.Close()
	remoteSampler.Close() // stop timer-based updates, we want to call them manually

	numOperations := 1000
	adaptiveSampler, err := NewAdaptiveSampler(
		&sampling.PerOperationSamplingStrategies{
			DefaultSamplingProbability: 1,
		},
		2000,
	)
	require.NoError(t, err)

	// Overwrite the sampler with an adaptive sampler
	remoteSampler.sampler = adaptiveSampler

	var wg sync.WaitGroup
	defer wg.Wait()
	wg.Add(2)

	isSampled := func(t *testing.T, remoteSampler *RemotelyControlledSampler, numOperations int, operationNamePrefix string) {
		for i := 0; i < numOperations; i++ {
			runtime.Gosched()
			span := &Span{
				operationName: fmt.Sprintf("%s%d", operationNamePrefix, i),
			}
			decision := remoteSampler.OnCreateSpan(span)
			assert.True(t, decision.sample)
		}
	}

	// Start 2 go routines that will simulate simultaneous calls to IsSampled
	go func() {
		defer wg.Done()
		isSampled(t, remoteSampler, numOperations, "a")
	}()
	go func() {
		defer wg.Done()
		isSampled(t, remoteSampler, numOperations, "b")
	}()
}
