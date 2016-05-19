package jaeger

import (
	"errors"
	"testing"
	"time"

	"github.com/uber/jaeger-client-go/testutils"
	"github.com/uber/jaeger-client-go/thrift/gen/sampling"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/thrift"
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
	assert.False(t, sampler.IsSampled(id1+10))
	assert.True(t, sampler.IsSampled(id1-20))
	sampler2, _ := NewProbabilisticSampler(0.5)
	assert.True(t, sampler.Equal(sampler2))
	assert.False(t, sampler.Equal(NewConstSampler(true)))
}

func TestProbabilisticSamplerPerformance(t *testing.T) {
	t.Skip("Skipped performance test")
	sampler, _ := NewProbabilisticSampler(0.01)
	rand := NewRand(8736823764)
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

	limiter := sampler.(*rateLimitingSampler).rateLimiter.(*rateLimiter)
	// stop time
	ts := time.Now()
	limiter.lastTick = ts
	limiter.timeNow = func() time.Time {
		return ts
	}
	assert.True(t, sampler.IsSampled(0))
	assert.True(t, sampler.IsSampled(0))
	assert.False(t, sampler.IsSampled(0))
	// move time 250ms forward, not enough credits to pay for one sample
	limiter.timeNow = func() time.Time {
		return ts.Add(time.Second / 4)
	}
	assert.False(t, limiter.CheckCredit(1.0))
	// move time 500ms forward, now enough credits to pay for one sample
	limiter.timeNow = func() time.Time {
		return ts.Add(time.Second/4 + time.Second/2)
	}
	assert.True(t, sampler.IsSampled(0))
	assert.False(t, sampler.IsSampled(0))
	// move time 5s forward, enough to accumulate credits for 10 samples, but it should still be capped at 2
	limiter.lastTick = ts
	limiter.timeNow = func() time.Time {
		return ts.Add(5 * time.Second)
	}
	assert.True(t, sampler.IsSampled(0))
	assert.True(t, sampler.IsSampled(0))
	assert.False(t, sampler.IsSampled(0))
	assert.False(t, sampler.IsSampled(0))
	assert.False(t, sampler.IsSampled(0))
}

func TestTCollectorControlledSampler(t *testing.T) {
	collector, err := testutils.StartMockTCollector()
	require.NoError(t, err)

	testRemoteControlledSampler(t,
		func(service string, m *Metrics) Sampler {
			return NewTCollectorControlledSampler(service, nil, /* init sampler */
				collector.Channel, m, NullLogger)
		},
		func(service string, strategy *sampling.SamplingStrategyResponse) {
			collector.AddSamplingStrategy(service, strategy)
		})
}

func TestRemoteControlledSamplerHTTP(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	testRemoteControlledSampler(t,
		func(service string, m *Metrics) Sampler {
			return NewHTTPRemoteControlledSampler(service, nil, /* init sampler */
				agent.SamplingServerAddr(), m, NullLogger)
		},
		func(service string, strategy *sampling.SamplingStrategyResponse) {
			agent.AddSamplingStrategy(service, strategy)
		})
}

func testRemoteControlledSampler(
	t *testing.T,
	factory func(service string, m *Metrics) Sampler,
	addStrategy func(service string, strategy *sampling.SamplingStrategyResponse),
) {
	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)

	samplerIface := factory("client app", metrics)
	assert.NotNil(t, samplerIface)
	sampler := samplerIface.(*remoteControlledSampler)

	initSampler, ok := sampler.sampler.(*probabilisticSampler)
	assert.True(t, ok)

	sampler.Close() // stop timer-based updates, we want to call them manually

	addStrategy("client app", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: 1.5, // bad value
		}})
	sampler.updateSampler()
	assert.Equal(t, map[string]int64{"jaeger.sampler|phase=parsing|state=failure": 1}, stats.GetCounterValues())
	_, ok = sampler.sampler.(*probabilisticSampler)
	assert.True(t, ok)
	assert.Equal(t, initSampler, sampler.sampler, "Sampler should not have been updated")

	addStrategy("client app", &sampling.SamplingStrategyResponse{
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
	_, ok = sampler.sampler.(*probabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated")

	id := uint64(1) << 62
	assert.True(t, sampler.IsSampled(id-10))
	assert.False(t, sampler.IsSampled(id+10))

	sampler.sampler = initSampler
	c := make(chan time.Time)
	sampler.lock.Lock()
	sampler.timer = &time.Ticker{C: c}
	sampler.lock.Unlock()
	go sampler.pollController()

	c <- time.Now() // force update based on timer
	time.Sleep(10 * time.Millisecond)
	sampler.Close()

	_, ok = sampler.sampler.(*probabilisticSampler)
	assert.True(t, ok)
	assert.NotEqual(t, initSampler, sampler.sampler, "Sampler should have been updated from timer")

	_, err := sampler.extractSampler(&sampling.SamplingStrategyResponse{})
	assert.Error(t, err)
	_, err = sampler.extractSampler(&sampling.SamplingStrategyResponse{
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{SamplingRate: 1.1}})
	assert.Error(t, err)
}

func TestSamplerQueryError(t *testing.T) {
	collector, err := testutils.StartMockTCollector()
	require.NoError(t, err)

	stats := NewInMemoryStatsCollector()
	metrics := NewMetrics(stats, nil)

	samplerIface := NewTCollectorControlledSampler("clientapp", nil, /* init sampler */
		collector.Channel, metrics, NullLogger)
	assert.NotNil(t, samplerIface)
	sampler := samplerIface.(*remoteControlledSampler)
	// override the actual handler
	sampler.manager.(*tcollectorSamplingManager).client = &fakeSamplingManager{}

	initSampler, ok := sampler.sampler.(*probabilisticSampler)
	assert.True(t, ok)

	sampler.Close() // stop timer-based updates, we want to call them manually

	sampler.updateSampler()
	assert.Equal(t, initSampler, sampler.sampler, "Sampler should not have been updated due to query error")
	assert.Equal(t, map[string]int64{"jaeger.sampler|phase=query|state=failure": 1}, stats.GetCounterValues())
}

type fakeSamplingManager struct{}

func (c *fakeSamplingManager) GetSamplingStrategy(ctx thrift.Context, serviceName string) (*sampling.SamplingStrategyResponse, error) {
	return nil, errors.New("query error")
}
