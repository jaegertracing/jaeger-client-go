package testutils

import (
	"sync"

	"github.com/uber/jaeger-client-go/thrift/gen/sampling"
)

func newSamplingManager() *samplingManager {
	return &samplingManager{
		sampling: make(map[string]*sampling.SamplingStrategyResponse),
	}
}

type samplingManager struct {
	sampling map[string]*sampling.SamplingStrategyResponse
	mutex    sync.Mutex
}

// GetSamplingStrategy implements handler method of sampling.SamplingManager
func (s *samplingManager) GetSamplingStrategy(serviceName string) (*sampling.SamplingStrategyResponse, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if strategy, ok := s.sampling[serviceName]; ok {
		return strategy, nil
	}
	return &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_PROBABILISTIC,
		ProbabilisticSampling: &sampling.ProbabilisticSamplingStrategy{
			SamplingRate: 0.01,
		}}, nil
}

// AddSamplingStrategy registers a sampling strategy for a service
func (s *samplingManager) AddSamplingStrategy(service string, strategy *sampling.SamplingStrategyResponse) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.sampling[service] = strategy
}
