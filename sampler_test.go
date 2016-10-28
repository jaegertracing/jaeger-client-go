package jaeger

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go/testutils"
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
	remote := &RemotelyControlledSampler{
		sampler: NewConstSampler(true),
	}
	tests := []struct {
		sampler  Sampler
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
		_, tags := test.sampler.IsSampled(0, testOperationName)
		count := 0
		for _, tag := range tags {
			if tag.key == SamplerTypeTagKey {
				assert.Equal(t, test.typeTag, tag.value)
				count++
			}
			if tag.key == SamplerParamTagKey {
				assert.Equal(t, test.paramTag, tag.value)
				count++
			}
		}
		assert.Equal(t, 2, count)
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
	sampled, tags := sampler.IsSampled(testMaxID+10, testOperationName)
	assert.False(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
	sampled, tags = sampler.IsSampled(testMaxID-20, testOperationName)
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
		id := uint64(rand.Int63())
		if sampled, _ := sampler.IsSampled(id, testOperationName); sampled {
			count++
		}
	}
	println("Sampled:", count, "rate=", float64(count)/float64(100000000))
	// Sampled: 999829 rate= 0.009998290
}

func TestRateLimitingSampler(t *testing.T) {
	sampler := NewRateLimitingSampler(2)
	sampler2 := NewRateLimitingSampler(2)
	sampler3 := NewRateLimitingSampler(3)
	assert.True(t, sampler.Equal(sampler2))
	assert.False(t, sampler.Equal(sampler3))
	assert.False(t, sampler.Equal(NewConstSampler(false)))
}

func TestGuaranteedThroughputProbabilisticSamplerUpdate(t *testing.T) {
	samplingRate := 0.5
	lowerBound := 2.0
	sampler, err := NewGuaranteedThroughputProbabilisticSampler(testOperationName, lowerBound, samplingRate)
	assert.NoError(t, err)

	assert.Equal(t, lowerBound, sampler.lowerBound)
	assert.Equal(t, samplingRate, sampler.samplingRate)

	newSamplingRate := 0.6
	newLowerBound := 1.0
	err = sampler.update(newLowerBound, newSamplingRate)
	assert.NoError(t, err)

	assert.Equal(t, newLowerBound, sampler.lowerBound)
	assert.Equal(t, newSamplingRate, sampler.samplingRate)

	newSamplingRate = 1.1
	err = sampler.update(newLowerBound, newSamplingRate)
	assert.Error(t, err)
}

func TestAdaptiveSampler(t *testing.T) {
	samplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: testDefaultSamplingProbability},
		},
	}
	strategies := &sampling.PerOperationSamplingStrategies{testDefaultSamplingProbability, 2.0, samplingRates}

	sampler, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	require.NoError(t, err)
	defer sampler.Close()

	sampled, tags := sampler.IsSampled(testMaxID-20, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)

	sampled, tags = sampler.IsSampled(testMaxID+10, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testLowerBoundExpectedTags, tags)

	sampled, tags = sampler.IsSampled(testMaxID+10, testOperationName)
	assert.False(t, sampled)

	// This operation is seen for the first time by the sampler
	sampled, tags = sampler.IsSampled(testMaxID, testFirstTimeOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
}

func TestAdaptiveSamplerErrors(t *testing.T) {
	samplingRate := -0.1
	lowerBound := 2.0
	samplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: samplingRate},
		},
	}
	strategies := &sampling.PerOperationSamplingStrategies{testDefaultSamplingProbability, lowerBound, samplingRates}

	_, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.Error(t, err)

	samplingRates[0].ProbabilisticSampling.SamplingRate = 1.1
	_, err = NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.Error(t, err)
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
	strategies := &sampling.PerOperationSamplingStrategies{testDefaultSamplingProbability, lowerBound, samplingRates}

	s, err := NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.NoError(t, err)

	sampler, ok := s.(*adaptiveSampler)
	assert.True(t, ok)
	assert.Equal(t, lowerBound, sampler.lowerBound)
	assert.Equal(t, testDefaultSamplingProbability, sampler.defaultSamplingProbability)
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
	strategies = &sampling.PerOperationSamplingStrategies{newDefaultSamplingProbability, newLowerBound, newSamplingRates}

	s, err = NewAdaptiveSampler(strategies, testDefaultMaxOperations)
	assert.NoError(t, err)

	sampler, ok = s.(*adaptiveSampler)
	assert.True(t, ok)
	assert.Equal(t, newLowerBound, sampler.lowerBound)
	assert.Equal(t, newDefaultSamplingProbability, sampler.defaultSamplingProbability)
	assert.Len(t, sampler.samplers, 2)
}

func initAgent(t *testing.T) (*testutils.MockAgent, *RemotelyControlledSampler, *InMemoryStatsCollector) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)

	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)

	initialSampler, _ := NewProbabilisticSampler(0.001)
	sampler := NewRemotelyControlledSampler(
		"client app",
		RemotelyControlledSamplerOptions.Metrics(metrics),
		RemotelyControlledSamplerOptions.HostPort(agent.SamplingServerAddr()),
		RemotelyControlledSamplerOptions.MaxOperations(testDefaultMaxOperations),
		RemotelyControlledSamplerOptions.InitialSampler(initialSampler),
		RemotelyControlledSamplerOptions.Logger(NullLogger),
	)
	sampler.Close() // stop timer-based updates, we want to call them manually

	return agent, sampler, stats
}

func TestRemotelyControlledSampler(t *testing.T) {
	agent, sampler, stats := initAgent(t)
	defer agent.Close()

	initSampler, ok := sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)

	agent.AddSamplingStrategy("client app", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: 1.5, // bad value
		}})
	sampler.updateSampler()
	assert.EqualValues(t, 1, stats.GetCounterValue("jaeger.sampler", "phase", "parsing", "state", "failure"))
	_, ok = sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.Equal(t, initSampler, sampler.sampler, "Sampler should not have been updated")

	agent.AddSamplingStrategy("client app", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: testDefaultSamplingProbability, // good value
		}})
	sampler.updateSampler()
	assertMetrics(t, stats, []expectedMetric{
		{[]string{"jaeger.sampler", "phase", "parsing", "state", "failure"}, 1},
		{[]string{"jaeger.sampler", "state", "retrieved"}, 1},
		{[]string{"jaeger.sampler", "state", "updated"}, 1},
	})

	_, ok = sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated")

	sampled, tags := sampler.IsSampled(testMaxID+10, testOperationName)
	assert.False(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
	sampled, tags = sampler.IsSampled(testMaxID-10, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)

	sampler.sampler = initSampler
	c := make(chan time.Time)
	sampler.Lock()
	sampler.timer = &time.Ticker{C: c}
	sampler.Unlock()
	go sampler.pollController()

	c <- time.Now() // force update based on timer
	time.Sleep(10 * time.Millisecond)
	sampler.Close()

	_, ok = sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated from timer")

	_, _, err := sampler.extractSampler(&sampling.SamplingStrategyResponse{})
	assert.Error(t, err)
	_, _, err = sampler.extractSampler(&sampling.SamplingStrategyResponse{
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 1.1}})
	assert.Error(t, err)
}

func TestUpdateSampler(t *testing.T) {
	agent, sampler, stats := initAgent(t)
	defer agent.Close()

	initSampler, ok := sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)

	tests := []struct {
		probabilities map[string]float64
		statTags      []string
		statCount     int64
		isErr         bool
	}{
		{
			map[string]float64{testOperationName: 1.1},
			[]string{"phase", "parsing", "state", "failure"},
			1,
			true,
		},
		{
			map[string]float64{testOperationName: testDefaultSamplingProbability},
			[]string{"state", "updated"},
			1,
			false,
		},
		{
			map[string]float64{
				testOperationName:          testDefaultSamplingProbability,
				testFirstTimeOperationName: testDefaultSamplingProbability,
			},
			[]string{"state", "updated"},
			2,
			false,
		},
		{
			map[string]float64{testOperationName: 1.1},
			[]string{"phase", "updating", "state", "failure"},
			1,
			true,
		},
		{
			map[string]float64{"new op": 1.1},
			[]string{"phase", "updating", "state", "failure"},
			2,
			true,
		},
	}

	for _, test := range tests {
		res := &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
			OperationSampling: &sampling.PerOperationSamplingStrategies{
				DefaultSamplingProbability:       testDefaultSamplingProbability,
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

		assert.EqualValues(t, test.statCount, stats.GetCounterValue("jaeger.sampler", test.statTags...))
		if test.isErr {
			continue
		}

		_, ok = sampler.sampler.(*adaptiveSampler)
		assert.True(t, ok)
		assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated")

		sampled, tags := sampler.IsSampled(testMaxID+10, testOperationName)
		assert.False(t, sampled)
		sampled, tags = sampler.IsSampled(testMaxID-10, testOperationName)
		assert.True(t, sampled)
		assert.Equal(t, testProbabilisticExpectedTags, tags)
	}
}

func TestMaxOperations(t *testing.T) {
	samplingRates := []*sampling.OperationSamplingStrategy{
		{
			Operation:             testOperationName,
			ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 0.1},
		},
	}
	strategies := &sampling.PerOperationSamplingStrategies{testDefaultSamplingProbability, 2.0, samplingRates}

	sampler, err := NewAdaptiveSampler(strategies, 1)
	assert.NoError(t, err)

	sampled, tags := sampler.IsSampled(testMaxID-10, testFirstTimeOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
}

func TestSamplerQueryError(t *testing.T) {
	agent, sampler, stats := initAgent(t)
	defer agent.Close()

	// override the actual handler
	sampler.manager = &fakeSamplingManager{}

	initSampler, ok := sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)

	sampler.Close() // stop timer-based updates, we want to call them manually

	sampler.updateSampler()
	assert.Equal(t, initSampler, sampler.sampler, "Sampler should not have been updated due to query error")
	assert.EqualValues(t, 1, stats.GetCounterValue("jaeger.sampler", "phase", "query", "state", "failure"))
}

type fakeSamplingManager struct{}

func (c *fakeSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("query error")
}
