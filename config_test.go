package jaeger

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewSamplerConst(t *testing.T) {
	constTests := []struct {
		param    float64
		decision bool
	}{{1, true}, {0, false}}
	for _, tst := range constTests {
		cfg := &SamplerConfig{Type: samplerTypeConst, Param: tst.param}
		s, err := cfg.NewSampler("x", nil)
		require.NoError(t, err)
		s1, ok := s.(*constSampler)
		require.True(t, ok, "converted to constSampler")
		require.Equal(t, tst.decision, s1.decision, "decision")
	}
}

func TestNewSamplerProbabilistic(t *testing.T) {
	constTests := []struct {
		param float64
		error bool
	}{{1.5, true}, {0.5, false}}
	for _, tst := range constTests {
		cfg := &SamplerConfig{Type: samplerTypeProbabilistic, Param: tst.param}
		s, err := cfg.NewSampler("x", nil)
		if tst.error {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			_, ok := s.(*probabilisticSampler)
			require.True(t, ok, "converted to probabilisticSampler")
		}
	}
}

func TestDefaultSampler(t *testing.T) {
	cfg := &SamplerConfig{}
	s, err := cfg.NewSampler("x", NewMetrics(NullStatsReporter, nil))
	require.NoError(t, err)
	rcs, ok := s.(*remoteControlledSampler)
	require.True(t, ok, "converted to remoteControlledSampler")
	rcs.Close()
}
