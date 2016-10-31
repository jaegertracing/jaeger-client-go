package config

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go"
)

func TestNewSamplerConst(t *testing.T) {
	constTests := []struct {
		param    float64
		decision bool
	}{{1, true}, {0, false}}
	for _, tst := range constTests {
		cfg := &SamplerConfig{Type: jaeger.SamplerTypeConst, Param: tst.param}
		s, err := cfg.NewSampler("x", nil)
		require.NoError(t, err)
		s1, ok := s.(*jaeger.ConstSampler)
		require.True(t, ok, "converted to constSampler")
		require.Equal(t, tst.decision, s1.Decision, "decision")
	}
}

func TestNewSamplerProbabilistic(t *testing.T) {
	constTests := []struct {
		param float64
		error bool
	}{{1.5, true}, {0.5, false}}
	for _, tst := range constTests {
		cfg := &SamplerConfig{Type: jaeger.SamplerTypeProbabilistic, Param: tst.param}
		s, err := cfg.NewSampler("x", nil)
		if tst.error {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			_, ok := s.(*jaeger.ProbabilisticSampler)
			require.True(t, ok, "converted to ProbabilisticSampler")
		}
	}
}

func TestDefaultSampler(t *testing.T) {
	cfg := &SamplerConfig{MaxOperations: 10}
	s, err := cfg.NewSampler("x", jaeger.NewMetrics(jaeger.NullStatsReporter, nil))
	require.NoError(t, err)
	rcs, ok := s.(*jaeger.RemotelyControlledSampler)
	require.True(t, ok, "converted to RemotelyControlledSampler")
	rcs.Close()
}
