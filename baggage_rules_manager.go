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

type BaggageRestrictionManager interface {
	IsValidBaggageKey(key string) (bool, int32)
}

type baggageRestrictionManager struct {
	sync.RWMutex

	serviceName string
	restrictionsMap map[string]int32
	timer       *time.Ticker
	manager     baggage.BaggageRestrictionManager
	pollStopped sync.WaitGroup
}

func NewBaggageRestrictionManager(serviceName string, manager baggage.BaggageRestrictionManager) BaggageRestrictionManager {
	m := &baggageRestrictionManager{
		serviceName: serviceName,
		restrictionsMap: make(map[string]int32),
		timer:       time.NewTicker(defaultRefreshInterval),
		manager:     manager,
	}
	m.updateRestrictions()
	go m.pollManager()
	return m
}

func (m *baggageRestrictionManager) IsValidBaggageKey(key string) (bool, int32) {
	m.RLock()
	defer m.RUnlock()
	if maxValueLength, ok := m.restrictionsMap[key]; ok {
		return true, maxValueLength
	}
	return false, 0
}

// Close implements Close() of Sampler.
func (m *baggageRestrictionManager) Close() {
	m.Lock()
	m.timer.Stop()
	m.Unlock()

	m.pollStopped.Wait()
}

func (m *baggageRestrictionManager) pollManager() {
	m.pollStopped.Add(1)
	defer m.pollStopped.Done()

	// in unit tests we re-assign the timer Ticker, so need to lock to avoid data races
	m.Lock()
	timer := m.timer
	m.Unlock()

	for range timer.C {
		m.updateRestrictions()
	}
}

func (m *baggageRestrictionManager) updateRestrictions() {
	restrictions, err := m.manager.GetBaggageRestrictions(m.serviceName)
	if err != nil {
		// TODO update failure metric
		// TODO this is pretty serious, almost retry worthy
		return
	}
	restrictionsMap := convertRestrictionsToMap(restrictions)
	m.Lock()
	defer m.Unlock()
	m.restrictionsMap = restrictionsMap
	// TODO update metrics
}

func convertRestrictionsToMap(restrictions []*baggage.BaggageRestriction) map[string]int32 {
	restrictionsMap := make(map[string]int32, len(restrictions))
	for _, restriction := range restrictions {
		restrictionsMap[restriction.BaggageKey] = restriction.MaxValueLength
	}
	return restrictionsMap
}
