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

var (
	testOperationName             = "op"
	testFirstTimeOperationName    = "firstTimeOp"
	testProbabilisticExpectedTags = []Tag{
		{"sampler.type", "probabilistic"},
		{"sampler.param", 0.5},
	}
	testLowerBoundExpectedTags = []Tag{
		{"sampler.type", "lowerbound"},
		{"sampler.param", 0.5},
	}
)

func TestProbabilisticSamplerErrors(t *testing.T) {
	_, err := NewProbabilisticSampler(-0.1)
	assert.Error(t, err)
	_, err = NewProbabilisticSampler(1.1)
	assert.Error(t, err)
}

func TestProbabilisticSampler(t *testing.T) {
	sampler, _ := NewProbabilisticSampler(0.5)
	id1 := uint64(1) << 62
	sampled, tags := sampler.IsSampled(id1+10, testOperationName)
	assert.False(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
	sampled, tags = sampler.IsSampled(id1-20, testOperationName)
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
		sampled, _ := sampler.IsSampled(id, testOperationName)
		if sampled {
			count++
		}
	}
	println("Sampled:", count, "rate=", float64(count)/float64(100000000))
	// Sampled: 999829 rate= 0.009998290
}

func TestRateLimitingSampler(t *testing.T) {
	sampler, _ := NewRateLimitingSampler(2)
	sampler2, _ := NewRateLimitingSampler(2)
	sampler3, _ := NewRateLimitingSampler(3)
	assert.True(t, sampler.Equal(sampler2))
	assert.False(t, sampler.Equal(sampler3))
	assert.False(t, sampler.Equal(NewConstSampler(false)))
}

func TestAdaptiveSampler(t *testing.T) {
	samplingRate := 0.5
	maxTracesPerSecond := 2.0
	samplingRates := map[string]*float64{
		testOperationName: &samplingRate,
	}
	sampler, err := NewAdaptiveSampler(maxTracesPerSecond, samplingRates)
	defer sampler.Close()
	require.NoError(t, err)

	id1 := uint64(1) << 62
	sampled, tags := sampler.IsSampled(id1-20, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)

	sampled, tags = sampler.IsSampled(id1+10, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testLowerBoundExpectedTags, tags)

	// This operation is the seen for the first time by the sampler
	sampled, tags = sampler.IsSampled(id1, testFirstTimeOperationName)
	assert.False(t, sampled)
	assert.Nil(t, tags)
}

func TestAdaptiveSamplerErrors(t *testing.T) {
	samplingRate := -0.1
	maxTracesPerSecond := 2.0
	samplingRates := map[string]*float64{
		testOperationName: &samplingRate,
	}
	_, err := NewAdaptiveSampler(maxTracesPerSecond, samplingRates)
	assert.Error(t, err)

	samplingRate = 1.1
	_, err = NewAdaptiveSampler(maxTracesPerSecond, samplingRates)
	assert.Error(t, err)
}

func TestAdaptiveSamplerEqual(t *testing.T) {
	samplingRateA := 0.5
	samplingRateB := 0.6
	maxTracesPerSecondA := 2.0
	maxTracesPerSecondB := 3.0

	samplingRates := map[string]*float64{
		testOperationName: &samplingRateA,
	}
	sampler, _ := NewAdaptiveSampler(maxTracesPerSecondA, samplingRates)

	tests := []struct {
		operation          string
		samplingRate       *float64
		maxTracesPerSecond float64
		equal              bool
	}{
		{testOperationName, &samplingRateA, maxTracesPerSecondA, true},
		{testFirstTimeOperationName, &samplingRateA, maxTracesPerSecondA, false},
		{testOperationName, nil, maxTracesPerSecondA, false},
		{testOperationName, &samplingRateA, maxTracesPerSecondB, false},
		{testOperationName, &samplingRateB, maxTracesPerSecondA, false},
	}

	for _, test := range tests {
		testSampler, _ := NewAdaptiveSampler(test.maxTracesPerSecond, map[string]*float64{
			test.operation: test.samplingRate,
		})
		assert.Equal(t, test.equal, sampler.Equal(testSampler))
	}

	// Test incompatible sampler types
	rateLimitingSampler, _ := NewRateLimitingSampler(2)
	assert.False(t, sampler.Equal(rateLimitingSampler))
}

func TestRemotelyControlledSampler(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)

	sampler := NewRemotelyControlledSampler("client app", nil, /* init sampler */
		agent.SamplingServerAddr(), metrics, NullLogger)

	initSampler, ok := sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)

	sampler.Close() // stop timer-based updates, we want to call them manually

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
			SamplingRate: 0.5, // good value
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

	id := uint64(1) << 62
	sampled, tags := sampler.IsSampled(id-10, testOperationName)
	assert.True(t, sampled)
	assert.Equal(t, testProbabilisticExpectedTags, tags)
	sampled, tags = sampler.IsSampled(id+10, testOperationName)
	assert.False(t, sampled)
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

	_, err = sampler.extractSampler(&sampling.SamplingStrategyResponse{})
	assert.Error(t, err)
	_, err = sampler.extractSampler(&sampling.SamplingStrategyResponse{
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 1.1}})
	assert.Error(t, err)
}

func TestSamplerQueryError(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)

	sampler := NewRemotelyControlledSampler("client app", nil, /* init sampler */
		agent.SamplingServerAddr(), metrics, NullLogger)

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
