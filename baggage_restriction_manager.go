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
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/uber/jaeger-client-go/thrift-gen/baggage"
	"github.com/uber/jaeger-client-go/utils"
)

const (
	defaultMaxValueLength = 2048
)

// BaggageRestrictionManager keeps track of valid baggage keys and their size restrictions.
type BaggageRestrictionManager interface {
	// IsValidBaggageKey returns whether the baggage key is valid given the restrictions
	// and the max baggage value length
	IsValidBaggageKey(key string) (bool, int32)
}

// DefaultBaggageRestrictionManager allows any baggage key.
type DefaultBaggageRestrictionManager struct{}

// IsValidBaggageKey implements BaggageRestrictionManager#IsValidBaggageKey
func (m DefaultBaggageRestrictionManager) IsValidBaggageKey(key string) (bool, int32) {
	return true, defaultMaxValueLength
}

type httpBaggageRestrictionsManager struct {
	serverURL string
}

func (s *httpBaggageRestrictionsManager) GetBaggageRestrictions(serviceName string) ([]*baggage.BaggageRestriction, error) {
	var out []*baggage.BaggageRestriction
	v := url.Values{}
	v.Set("service", serviceName)
	if err := utils.GetJSON(s.serverURL+"/baggage?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

type baggageRestrictionManager struct {
	baggageRestrictionManagerOptions
	sync.RWMutex

	serviceName     string
	restrictionsMap map[string]int32
	timer           *time.Ticker
	manager         baggage.BaggageRestrictionManager
	pollStopped     sync.WaitGroup
	stopPoll        chan struct{}
}

// NewBaggageRestrictionManager returns a BaggageRestrictionManager that polls the agent for the latest
// baggage restrictions.
func NewBaggageRestrictionManager(
	serviceName string,
	options ...BaggageRestrictionManagerOption,
) BaggageRestrictionManager {
	opts := applyOptions(options...)
	m := &baggageRestrictionManager{
		serviceName:                      serviceName,
		baggageRestrictionManagerOptions: opts,
		restrictionsMap:                  make(map[string]int32),
		timer:                            time.NewTicker(opts.refreshInterval),
		manager:                          &httpBaggageRestrictionsManager{serverURL: opts.baggageRestrictionManagerServerURL},
		stopPoll:                         make(chan struct{}),
	}
	m.updateRestrictions()
	m.pollStopped.Add(1)
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
	close(m.stopPoll)
	m.pollStopped.Wait()
}

func (m *baggageRestrictionManager) pollManager() {
	defer m.pollStopped.Done()
	defer m.timer.Stop()

	// in unit tests we re-assign the timer Ticker, so need to lock to avoid data races
	m.Lock()
	timer := m.timer
	m.Unlock()

	for {
		select {
		case <-timer.C:
			m.updateRestrictions()
		case <-m.stopPoll:
			return
		}
	}
}

func (m *baggageRestrictionManager) updateRestrictions() {
	restrictions, err := m.manager.GetBaggageRestrictions(m.serviceName)
	if err != nil {
		// TODO this is pretty serious, almost retry worthy
		m.metrics.BaggageManagerUpdateFailure.Inc(1)
		m.logger.Error(fmt.Sprintf("Failed to update baggage restrictions: %s", err.Error()))
		return
	}
	restrictionsMap := convertRestrictionsToMap(restrictions)
	m.metrics.BaggageManagerUpdateSuccess.Inc(1)
	m.Lock()
	defer m.Unlock()
	m.restrictionsMap = restrictionsMap
}

func convertRestrictionsToMap(restrictions []*baggage.BaggageRestriction) map[string]int32 {
	restrictionsMap := make(map[string]int32, len(restrictions))
	for _, restriction := range restrictions {
		restrictionsMap[restriction.BaggageKey] = restriction.MaxValueLength
	}
	return restrictionsMap
}
