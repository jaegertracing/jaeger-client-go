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

	thrift "github.com/uber/jaeger-client-go/thrift-gen/baggage"
	"github.com/uber/jaeger-client-go/utils"
)

type httpBaggageRestrictionManagerProxy struct {
	serverURL string
}

func (s *httpBaggageRestrictionManagerProxy) GetBaggageRestrictions(serviceName string) ([]*thrift.BaggageRestriction, error) {
	var out []*thrift.BaggageRestriction
	v := url.Values{}
	v.Set("service", serviceName)
	if err := utils.GetJSON(fmt.Sprintf("%s?%s", s.serverURL, v.Encode()), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// remoteRestrictionManager manages baggage restrictions by retrieving baggage restrictions from agent
type remoteRestrictionManager struct {
	mux             sync.RWMutex
	serviceName     string
	metrics         *Metrics
	logger          Logger
	refreshInterval time.Duration
	setters         map[string]baggageSetter
	thriftProxy     thrift.BaggageRestrictionManager
	pollStopped     sync.WaitGroup
	stopPoll        chan struct{}
	invalidSetter   baggageSetter
	validSetter     baggageSetter
	failClosed      bool

	// Determines if the manager has successfully retrieved baggage restrictions from agent
	initialized bool
}

// newRemoteRestrictionManager returns a BaggageRestrictionManager that polls the agent for the latest
// baggage restrictions.
func newRemoteRestrictionManager(serviceName string, cfg *BaggageRestrictionsConfig, metrics *Metrics, logger Logger) *remoteRestrictionManager {
	// TODO there is a developing use case where a single tracer can generate traces on behalf of many services.
	// restrictionsMap will need to exist per service
	m := &remoteRestrictionManager{
		serviceName:     serviceName,
		setters:         make(map[string]baggageSetter),
		thriftProxy:     &httpBaggageRestrictionManagerProxy{serverURL: cfg.ServerURL},
		stopPoll:        make(chan struct{}),
		invalidSetter:   newInvalidBaggageSetter(metrics),
		validSetter:     newDefaultBaggageSetter(defaultMaxValueLength, metrics),
		metrics:         metrics,
		logger:          logger,
		refreshInterval: cfg.RefreshInterval,
		failClosed:      cfg.DenyBaggageOnInitializationFailure,
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

// isReady returns true if the manager has retrieved baggage restrictions from the remote source.
func (m *remoteRestrictionManager) isReady() bool {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return m.initialized
}

func (m *remoteRestrictionManager) setBaggage(span *Span, key, value string) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	setter := m.invalidSetter
	if !m.initialized {
		if !m.failClosed {
			setter = m.validSetter
		}
	}
	if serviceBaggageSetter, ok := m.setters[key]; ok {
		setter = serviceBaggageSetter
	}
	setter.setBaggage(span, key, value)
}

// Close stops remote polling and closes the RemoteRestrictionManager.
func (m *remoteRestrictionManager) Close() {
	close(m.stopPoll)
	m.pollStopped.Wait()
}

func (m *remoteRestrictionManager) pollManager() {
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

func (m *remoteRestrictionManager) updateRestrictions() error {
	restrictions, err := m.thriftProxy.GetBaggageRestrictions(m.serviceName)
	if err != nil {
		m.metrics.BaggageRestrictionsUpdateFailure.Inc(1)
		return err
	}
	setters := m.generateSetters(restrictions)
	m.metrics.BaggageRestrictionsUpdateSuccess.Inc(1)
	m.mux.Lock()
	defer m.mux.Unlock()
	m.initialized = true
	m.setters = setters
	return nil
}

func (m *remoteRestrictionManager) generateSetters(restrictions []*thrift.BaggageRestriction) map[string]baggageSetter {
	setters := make(map[string]baggageSetter, len(restrictions))
	for _, restriction := range restrictions {
		setters[restriction.BaggageKey] = newDefaultBaggageSetter(int(restriction.MaxValueLength), m.metrics)
	}
	return setters
}
