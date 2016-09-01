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

func TestSamplerTags(t *testing.T) {
	prob, err := NewProbabilisticSampler(0.1)
	require.NoError(t, err)
	rate, err := NewRateLimitingSampler(0.1)
	require.NoError(t, err)
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
		tags := test.sampler.getTags()
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
	id1 := uint64(1) << 62
	assert.False(t, sampler.IsSampled(id1+10))
	assert.True(t, sampler.IsSampled(id1-20))
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
		if sampler.IsSampled(id) {
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
	assert.Equal(t, map[string]int64{"jaeger.sampler|phase=parsing|state=failure": 1}, stats.GetCounterValues())
	_, ok = sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.Equal(t, initSampler, sampler.sampler, "Sampler should not have been updated")

	agent.AddSamplingStrategy("client app", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: 0.5, // good value
		}})
	sampler.updateSampler()
	assert.Equal(t, map[string]int64{
		"jaeger.sampler|phase=parsing|state=failure": 1,
		"jaeger.sampler|state=retrieved":             1,
		"jaeger.sampler|state=updated":               1,
	}, stats.GetCounterValues())
	_, ok = sampler.sampler.(*ProbabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated")

	id := uint64(1) << 62
	assert.True(t, sampler.IsSampled(id-10))
	assert.False(t, sampler.IsSampled(id+10))

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
	assert.Equal(t, map[string]int64{"jaeger.sampler|phase=query|state=failure": 1}, stats.GetCounterValues())
}

type fakeSamplingManager struct{}

func (c *fakeSamplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("query error")
}
