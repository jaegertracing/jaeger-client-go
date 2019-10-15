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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	mTestutils "github.com/uber/jaeger-lib/metrics/metricstest"

	"github.com/uber/jaeger-client-go/log"
	"github.com/uber/jaeger-client-go/testutils"
	"github.com/uber/jaeger-client-go/thrift-gen/sampling"
)

func TestApplySamplerOptions(t *testing.T) {
	options := new(samplerOptions).applyOptionsAndDefaults()
	sampler, ok := options.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.Equal(t, 0.001, sampler.samplingRate)

	assert.NotNil(t, options.logger)
	assert.NotZero(t, options.maxOperations)
	assert.NotEmpty(t, options.samplingServerURL)
	assert.NotNil(t, options.metrics)
	assert.NotZero(t, options.samplingRefreshInterval)
}

func initAgent(t *testing.T) (*testutils.MockAgent, *RemotelyControlledSampler, *mTestutils.Factory) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)

	metricsFactory := mTestutils.NewFactory(0)
	metrics := NewMetrics(metricsFactory, nil)

	initialSampler, _ := NewProbabilisticSampler(0.001)
	sampler := NewRemotelyControlledSampler(
		"client app",
		SamplerOptions.Metrics(metrics),
		SamplerOptions.SamplingServerURL("http://"+agent.SamplingServerAddr()),
		SamplerOptions.MaxOperations(testDefaultMaxOperations),
		SamplerOptions.InitialSampler(initialSampler),
		SamplerOptions.Logger(log.NullLogger),
		SamplerOptions.SamplingRefreshInterval(time.Minute),
	)
	sampler.Close() // stop timer-based updates, we want to call them manually

	return agent, sampler, metricsFactory
}

func makeSpan(id uint64, operationName string) *Span {
	return &Span{
		context: SpanContext{
			traceID:       TraceID{Low: id},
			samplingState: new(samplingState),
		},
		operationName: operationName,
	}
}

func TestRemotelyControlledSampler(t *testing.T) {
	agent, remoteSampler, metricsFactory := initAgent(t)
	defer agent.Close()

	initSampler, ok := remoteSampler.getSampler().(*ProbabilisticSampler)
	assert.True(t, ok)

	agent.AddSamplingStrategy("client app",
		getSamplingStrategyResponse(sampling.SamplingStrategyType_PROBABILISTIC, testDefaultSamplingProbability))
	remoteSampler.updateSampler()
	metricsFactory.AssertCounterMetrics(t, []mTestutils.ExpectedMetric{
		{Name: "jaeger.tracer.sampler_queries", Tags: map[string]string{"result": "ok"}, Value: 1},
		{Name: "jaeger.tracer.sampler_updates", Tags: map[string]string{"result": "ok"}, Value: 1},
	}...)
	s1, ok := remoteSampler.getSampler().(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, s1, "Sampler should have been updated")

	decision := remoteSampler.OnCreateSpan(makeSpan(testMaxID+10, testOperationName))
	assert.False(t, decision.sample)
	assert.Equal(t, testProbabilisticExpectedTags, decision.tags)
	decision = remoteSampler.OnCreateSpan(makeSpan(testMaxID-10, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, testProbabilisticExpectedTags, decision.tags)

	remoteSampler.setSampler(initSampler)

	c := make(chan time.Time)
	ticker := &time.Ticker{C: c}
	go remoteSampler.pollControllerWithTicker(ticker)

	c <- time.Now() // force update based on timer
	time.Sleep(10 * time.Millisecond)
	remoteSampler.Close()

	s2, ok := remoteSampler.getSampler().(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, s2, "Sampler should have been updated from timer")

	assert.True(t, remoteSampler.Equal(remoteSampler))
}

func generateSamplerTags(key string, value interface{}) []Tag {
	return []Tag{
		{"sampler.type", key},
		{"sampler.param", value},
	}
}

func TestRemotelyControlledSampler_updateSampler(t *testing.T) {
	tests := []struct {
		probabilities              map[string]float64
		defaultProbability         float64
		expectedDefaultProbability float64
		expectedTags               []Tag
	}{
		{
			probabilities:              map[string]float64{testOperationName: 1.1},
			defaultProbability:         testDefaultSamplingProbability,
			expectedDefaultProbability: testDefaultSamplingProbability,
			expectedTags:               generateSamplerTags("probabilistic", 1.0),
		},
		{
			probabilities:              map[string]float64{testOperationName: testDefaultSamplingProbability},
			defaultProbability:         testDefaultSamplingProbability,
			expectedDefaultProbability: testDefaultSamplingProbability,
			expectedTags:               testProbabilisticExpectedTags,
		},
		{
			probabilities: map[string]float64{
				testOperationName:          testDefaultSamplingProbability,
				testFirstTimeOperationName: testDefaultSamplingProbability,
			},
			defaultProbability:         testDefaultSamplingProbability,
			expectedDefaultProbability: testDefaultSamplingProbability,
			expectedTags:               testProbabilisticExpectedTags,
		},
		{
			probabilities:              map[string]float64{"new op": 1.1},
			defaultProbability:         testDefaultSamplingProbability,
			expectedDefaultProbability: testDefaultSamplingProbability,
			expectedTags:               testProbabilisticExpectedTags,
		},
		{
			probabilities:              map[string]float64{"new op": 1.1},
			defaultProbability:         1.1,
			expectedDefaultProbability: 1.0,
			expectedTags:               generateSamplerTags("probabilistic", 1.0),
		},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("test_%d", i), func(t *testing.T) {
			agent, sampler, metricsFactory := initAgent(t)
			defer agent.Close()

			initSampler, ok := sampler.sampler.(*ProbabilisticSampler)
			assert.True(t, ok)

			res := &sampling.SamplingStrategyResponse{
				StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
				OperationSampling: &sampling.PerOperationSamplingStrategies{
					DefaultSamplingProbability:       test.defaultProbability,
					DefaultLowerBoundTracesPerSecond: 0.001,
				},
			}
			for opName, prob := range test.probabilities {
				res.OperationSampling.PerOperationStrategies = append(res.OperationSampling.PerOperationStrategies,
					&sampling.OperationSamplingStrategy{
						Operation: opName,
						ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
							SamplingRate: prob,
						},
					},
				)
			}

			agent.AddSamplingStrategy("client app", res)
			sampler.updateSampler()

			metricsFactory.AssertCounterMetrics(t,
				mTestutils.ExpectedMetric{
					Name: "jaeger.tracer.sampler_updates", Tags: map[string]string{"result": "ok"}, Value: 1,
				},
			)

			s, ok := sampler.sampler.(*AdaptiveSampler)
			assert.True(t, ok)
			assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated")
			assert.Equal(t, test.expectedDefaultProbability, s.defaultSampler.SamplingRate())

			// First call is always sampled
			decision := sampler.OnCreateSpan(makeSpan(testMaxID+10, testOperationName))
			assert.True(t, decision.sample)

			decision = sampler.OnCreateSpan(makeSpan(testMaxID-10, testOperationName))
			assert.True(t, decision.sample)
			assert.Equal(t, test.expectedTags, decision.tags)
		})
	}
}

func TestRemotelyControlledSampler_updateDefaultRate(t *testing.T) {
	agent, sampler, _ := initAgent(t)
	defer agent.Close()

	res := &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		OperationSampling: &sampling.PerOperationSamplingStrategies{
			DefaultSamplingProbability: 0.5,
		},
	}
	agent.AddSamplingStrategy("client app", res)
	sampler.updateSampler()

	// Check what rate we get for a specific operation
	decision := sampler.OnCreateSpan(makeSpan(0, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, generateSamplerTags("probabilistic", 0.5), decision.tags)

	// Change the default and update
	res.OperationSampling.DefaultSamplingProbability = 0.1
	sampler.updateSampler()

	// Check sampling rate has changed
	decision = sampler.OnCreateSpan(makeSpan(0, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, generateSamplerTags("probabilistic", 0.1), decision.tags)

	// Add an operation-specific rate
	res.OperationSampling.PerOperationStrategies = []*sampling.OperationSamplingStrategy{{
		Operation: testOperationName,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: 0.2,
		},
	}}
	sampler.updateSampler()

	// Check we get the requested rate
	decision = sampler.OnCreateSpan(makeSpan(0, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, generateSamplerTags("probabilistic", 0.2), decision.tags)

	// Now remove the operation-specific rate
	res.OperationSampling.PerOperationStrategies = nil
	sampler.updateSampler()

	// Check we get the default rate
	assert.True(t, decision.sample)
	decision = sampler.OnCreateSpan(makeSpan(0, testOperationName))
	assert.True(t, decision.sample)
	assert.Equal(t, generateSamplerTags("probabilistic", 0.1), decision.tags)
}

func TestSamplerQueryError(t *testing.T) {
	agent, sampler, metricsFactory := initAgent(t)
	defer agent.Close()

	// override the actual handler
	sampler.manager = &fakeSamplingManager{}

	initSampler, ok := sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)

	sampler.Close() // stop timer-based updates, we want to call them manually

	sampler.updateSampler()
	assert.Equal(t, initSampler, sampler.sampler, "Sampler should not have been updated due to query error")

	metricsFactory.AssertCounterMetrics(t,
		mTestutils.ExpectedMetric{Name: "jaeger.tracer.sampler_queries", Tags: map[string]string{"result": "err"}, Value: 1},
	)
}

type fakeSamplingManager struct{}

func (c *fakeSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("query error")
}

func TestRemotelyControlledSampler_updateSamplerFromAdaptiveSampler(t *testing.T) {
	agent, remoteSampler, metricsFactory := initAgent(t)
	defer agent.Close()
	remoteSampler.Close() // close the second time (initAgent already called Close)

	strategies := &sampling.PerOperationSamplingStrategies{
		DefaultSamplingProbability:       testDefaultSamplingProbability,
		DefaultLowerBoundTracesPerSecond: 1.0,
	}

	adaptiveSampler, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	require.NoError(t, err)

	// Overwrite the sampler with an adaptive sampler
	remoteSampler.sampler = adaptiveSampler

	agent.AddSamplingStrategy("client app",
		getSamplingStrategyResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.5))
	remoteSampler.updateSampler()

	// Sampler should have been updated to probabilistic
	_, ok := remoteSampler.sampler.(*ProbabilisticSampler)
	require.True(t, ok)

	// Overwrite the sampler with an adaptive sampler
	remoteSampler.sampler = adaptiveSampler

	agent.AddSamplingStrategy("client app",
		getSamplingStrategyResponse(sampling.SamplingStrategyType_RATE_LIMITING, 1))
	remoteSampler.updateSampler()

	// Sampler should have been updated to ratelimiting
	_, ok = remoteSampler.sampler.(*RateLimitingSampler)
	require.True(t, ok)

	// Overwrite the sampler with an adaptive sampler
	remoteSampler.sampler = adaptiveSampler

	// Update existing adaptive sampler
	agent.AddSamplingStrategy("client app", &sampling.SamplingStrategyResponse{OperationSampling: strategies})
	remoteSampler.updateSampler()

	metricsFactory.AssertCounterMetrics(t,
		mTestutils.ExpectedMetric{Name: "jaeger.tracer.sampler_queries", Tags: map[string]string{"result": "ok"}, Value: 3},
		mTestutils.ExpectedMetric{Name: "jaeger.tracer.sampler_updates", Tags: map[string]string{"result": "ok"}, Value: 3},
	)
}

func TestRemotelyControlledSampler_updateRateLimitingOrProbabilisticSampler(t *testing.T) {
	probabilisticSampler, err := NewProbabilisticSampler(0.002)
	require.NoError(t, err)
	otherProbabilisticSampler, err := NewProbabilisticSampler(0.003)
	require.NoError(t, err)
	maxProbabilisticSampler, err := NewProbabilisticSampler(1.0)
	require.NoError(t, err)

	rateLimitingSampler := NewRateLimitingSampler(2)
	otherRateLimitingSampler := NewRateLimitingSampler(3)

	testCases := []struct {
		res                  *sampling.SamplingStrategyResponse
		initSampler          SamplerV2
		expectedSampler      Sampler
		shouldErr            bool
		referenceEquivalence bool
		caption              string
	}{
		{
			res:                  getSamplingStrategyResponse(sampling.SamplingStrategyType_PROBABILISTIC, 1.5),
			initSampler:          probabilisticSampler,
			expectedSampler:      maxProbabilisticSampler,
			shouldErr:            false,
			referenceEquivalence: false,
			caption:              "invalid probabilistic strategy",
		},
		{
			res:                  getSamplingStrategyResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.002),
			initSampler:          probabilisticSampler,
			expectedSampler:      probabilisticSampler,
			shouldErr:            false,
			referenceEquivalence: true,
			caption:              "unchanged probabilistic strategy",
		},
		{
			res:                  getSamplingStrategyResponse(sampling.SamplingStrategyType_PROBABILISTIC, 0.003),
			initSampler:          probabilisticSampler,
			expectedSampler:      otherProbabilisticSampler,
			shouldErr:            false,
			referenceEquivalence: false,
			caption:              "valid probabilistic strategy",
		},
		{
			res:                  getSamplingStrategyResponse(sampling.SamplingStrategyType_RATE_LIMITING, 2),
			initSampler:          rateLimitingSampler,
			expectedSampler:      rateLimitingSampler,
			shouldErr:            false,
			referenceEquivalence: true,
			caption:              "unchanged rate limiting strategy",
		},
		{
			res:                  getSamplingStrategyResponse(sampling.SamplingStrategyType_RATE_LIMITING, 3),
			initSampler:          rateLimitingSampler,
			expectedSampler:      otherRateLimitingSampler,
			shouldErr:            false,
			referenceEquivalence: false,
			caption:              "valid rate limiting strategy",
		},
		{
			res:                  &sampling.SamplingStrategyResponse{},
			initSampler:          rateLimitingSampler,
			expectedSampler:      rateLimitingSampler,
			shouldErr:            true,
			referenceEquivalence: true,
			caption:              "invalid strategy",
		},
	}

	for _, tc := range testCases {
		testCase := tc // capture loop var
		t.Run(testCase.caption, func(t *testing.T) {
			remoteSampler := &RemotelyControlledSampler{samplerOptions: samplerOptions{sampler: testCase.initSampler}}
			err := remoteSampler.updateRateLimitingOrProbabilisticSampler(testCase.res)
			if testCase.shouldErr {
				require.Error(t, err)
			}
			if testCase.referenceEquivalence {
				assert.Equal(t, testCase.expectedSampler, remoteSampler.sampler)
			} else {
				assert.True(t, testCase.expectedSampler.Equal(remoteSampler.sampler.(Sampler)))
			}
		})
	}
}

func getSamplingStrategyResponse(strategyType sampling.SamplingStrategyType, value float64) *sampling.SamplingStrategyResponse {
	if strategyType == sampling.SamplingStrategyType_PROBABILISTIC {
		return &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
				SamplingRate: value,
			},
		}
	}
	if strategyType == sampling.SamplingStrategyType_RATE_LIMITING {
		return &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: int16(value),
			},
		}
	}
	return nil
}

func TestRemotelyControlledSampler_printErrorForBrokenUpstream(t *testing.T) {
	logger := &log.BytesBufferLogger{}
	sampler := NewRemotelyControlledSampler(
		"client app",
		SamplerOptions.Logger(logger),
	)
	sampler.Close() // stop timer-based updates, we want to call them manually
	sampler.updateSampler()

	assert.True(t, strings.Contains(logger.String(), "Unable to query sampling strategy:"))
}
