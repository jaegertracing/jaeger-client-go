// Copyright (c) 2017 Uber Technologies, Inc.
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

func TestDefaultConfig(t *testing.T) {
	cfg := Configuration{}
	_, _, err := cfg.New("", jaeger.NullStatsReporter)
	require.EqualError(t, err, "no service name provided")

	_, closer, err := cfg.New("testService", jaeger.NullStatsReporter)
	defer closer.Close()
	require.NoError(t, err)
}
