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

package baggage

import (
	"fmt"
	"net/url"
	"sync"
	"time"

	thrift "github.com/uber/jaeger-client-go/thrift-gen/baggage"
	"github.com/uber/jaeger-client-go/utils"
)

const (
	defaultMaxValueLength = 2048
)

type httpRestrictionProxy struct {
	serverURL string
}

func (s *httpRestrictionProxy) GetBaggageRestrictions(serviceName string) ([]*thrift.BaggageRestriction, error) {
	var out []*thrift.BaggageRestriction
	v := url.Values{}
	v.Set("service", serviceName)
	if err := utils.GetJSON(s.serverURL+"?"+v.Encode(), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RemoteRestrictionManager manages baggage restrictions by retrieving baggage restrictions from agent
type RemoteRestrictionManager struct {
	options

	mux             sync.RWMutex
	serviceName     string
	restrictionsMap map[string]int
	thriftProxy     thrift.BaggageRestrictionManager
	pollStopped     sync.WaitGroup
	stopPoll        chan struct{}

	// Determines if the manager has successfully retrieved baggage restrictions from agent
	initialized bool
}

// NewRemoteRestrictionManager returns a BaggageRestrictionManager that polls the agent for the latest
// baggage restrictions.
func NewRemoteRestrictionManager(serviceName string, options ...Option) *RemoteRestrictionManager {
	// TODO there is a developing use case where a single tracer can generate traces on behalf of many services.
	// restrictionsMap will need to exist per service
	opts := applyOptions(options...)
	m := &RemoteRestrictionManager{
		serviceName:     serviceName,
		options:         opts,
		restrictionsMap: make(map[string]int),
		thriftProxy:     &httpRestrictionProxy{serverURL: opts.serverURL},
		stopPoll:        make(chan struct{}),
	}
	if err := m.updateRestrictions(); err != nil {
		m.logger.Error(fmt.Sprintf("Failed to initialize baggage restrictions: %s", err.Error()))
	} else {
		m.initialized = true
	}
	m.pollStopped.Add(1)
	go m.pollManager()
	return m
}

// IsValidBaggageKey implements BaggageRestrictionManager#IsValidBaggageKey
func (m *RemoteRestrictionManager) IsValidBaggageKey(key string) (bool, int) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	if !m.initialized {
		if m.failClosed {
			return false, 0
		}
		return true, defaultMaxValueLength
	}
	if maxValueLength, ok := m.restrictionsMap[key]; ok {
		return true, maxValueLength
	}
	return false, 0
}

// Close stops remote polling and closes the RemoteRestrictionManager.
func (m *RemoteRestrictionManager) Close() {
	close(m.stopPoll)
	m.pollStopped.Wait()
}

func (m *RemoteRestrictionManager) pollManager() {
	defer m.pollStopped.Done()
	ticker := time.NewTicker(m.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.updateRestrictions(); err != nil {
				m.logger.Error(fmt.Sprintf("Failed to update baggage restrictions: %s", err.Error()))
			}
		case <-m.stopPoll:
			return
		}
	}
}

func (m *RemoteRestrictionManager) updateRestrictions() error {
	restrictions, err := m.thriftProxy.GetBaggageRestrictions(m.serviceName)
	if err != nil {
		m.metrics.BaggageRestrictionsUpdateFailure.Inc(1)
		return err
	}
	restrictionsMap := convertRestrictionsToMap(restrictions)
	m.metrics.BaggageRestrictionsUpdateSuccess.Inc(1)
	m.mux.Lock()
	defer m.mux.Unlock()
	m.initialized = true
	m.restrictionsMap = restrictionsMap
	return nil
}

func convertRestrictionsToMap(restrictions []*thrift.BaggageRestriction) map[string]int {
	restrictionsMap := make(map[string]int, len(restrictions))
	for _, restriction := range restrictions {
		restrictionsMap[restriction.BaggageKey] = int(restriction.MaxValueLength)
	}
	return restrictionsMap
}
