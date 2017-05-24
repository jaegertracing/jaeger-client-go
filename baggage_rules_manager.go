package jaeger

import (
	"sync"
	"time"

	"github.com/uber/jaeger-client-go/thrift-gen/baggage"
)

const (
	defaultRefreshInterval = time.Minute
)

type BaggageRulesManager interface {
	IsValidBaggageKey(key string) (bool, int32)
}

type baggageRulesManager struct {
	sync.RWMutex

	baggageSize map[string]int32
	timer *time.Ticker
	manager     baggage.Baggage
	pollStopped sync.WaitGroup
}

func NewBaggageRulesManager() BaggageRulesManager {
	m := &baggageRulesManager{
		baggageSize: make(map[string]int32),
		timer:          time.NewTicker(defaultRefreshInterval),
	}
	go m.pollController()
	return m
}

func (m *baggageRulesManager) pollController() {
	// in unit tests we re-assign the timer Ticker, so need to lock to avoid data races
	m.Lock()
	timer := m.timer
	m.Unlock()

	for range timer.C {
		m.updateRules()
	}
	//s.pollStopped.Add(1)
}

func (m *baggageRulesManager) updateRules() {

	GetBaggageRules(serviceName string) (r []*BaggageRule, err error)

	//res, err := s.manager.GetSamplingStrategy(s.serviceName)
	//if err != nil {
	//	s.metrics.SamplerQueryFailure.Inc(1)
	//	return
	//}
	//s.Lock()
	//defer s.Unlock()
	//
	//s.metrics.SamplerRetrieved.Inc(1)
	//if strategies := res.GetOperationSampling(); strategies != nil {
	//	s.updateAdaptiveSampler(strategies)
	//} else {
	//	err = s.updateRateLimitingOrProbabilisticSampler(res)
	//}
	//if err != nil {
	//	s.metrics.SamplerUpdateFailure.Inc(1)
	//	s.logger.Infof("Unable to handle sampling strategy response %+v. Got error: %v", res, err)
	//	return
	//}
	//s.metrics.SamplerUpdated.Inc(1)
}

func (m *baggageRulesManager) Init() error {
	m.Lock()
	defer m.Unlock()
	// TODO get the baggage rules from agent
	return nil
}
