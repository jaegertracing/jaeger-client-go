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

	serviceName string
	baggageSize map[string]int32
	timer       *time.Ticker
	manager     baggage.BaggageManager
	pollStopped sync.WaitGroup
}

func NewBaggageRulesManager(serviceName string, manager baggage.BaggageManager) BaggageRulesManager {
	m := &baggageRulesManager{
		serviceName: serviceName,
		baggageSize: make(map[string]int32),
		timer:       time.NewTicker(defaultRefreshInterval),
		manager:     manager,
	}
	go m.pollManager()
	return m
}

func (m *baggageRulesManager) IsValidBaggageKey(key string) (bool, int32) {
	m.RLock()
	defer m.RUnlock()
	if size, ok := m.baggageSize[key]; ok {
		return true, size
	}
	return false, 0
}

// Close implements Close() of Sampler.
func (m *baggageRulesManager) Close() {
	m.Lock()
	m.timer.Stop()
	m.Unlock()

	m.pollStopped.Wait()
}

func (m *baggageRulesManager) pollManager() {
	m.pollStopped.Add(1)
	defer m.pollStopped.Done()

	// in unit tests we re-assign the timer Ticker, so need to lock to avoid data races
	m.Lock()
	timer := m.timer
	m.Unlock()

	for range timer.C {
		m.updateRules()
	}
}

func (m *baggageRulesManager) updateRules() {
	rules, err := m.manager.GetBaggageRules(m.serviceName)
	if err != nil {
		// TODO update failure metric
		// TODO this is pretty serious, almost retry worthy
		return
	}
	rulesMap := convertRulesToMap(rules)
	m.Lock()
	defer m.Unlock()
	m.baggageSize = rulesMap
	// TODO update metrics
}

func convertRulesToMap(rules []*baggage.BaggageRule) map[string]int32 {
	rulesMap := make(map[string]int32, len(rules))
	for _, rule := range rules {
		rulesMap[rule.Key] = rule.Size
	}
	return rulesMap
}
