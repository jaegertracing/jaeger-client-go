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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/testutils"
	"github.com/uber/jaeger-client-go/thrift-gen/baggage"
)

func TestNewRemoteRestrictionManager(t *testing.T) {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)
	defer agent.Close()

	service := "svc"
	expectedKey := "key"
	expectedSize := int32(10)

	agent.AddBaggageRestrictions(service, []*baggage.BaggageRestriction{
		{BaggageKey: expectedKey, MaxValueLength: expectedSize},
	})

	mgr, err := NewRemoteRestrictionManager(
		service,
		Options.RefreshInterval(10*time.Millisecond),
		Options.ServerURL("http://"+agent.ServerAddr()+"/baggageRestrictions"),
	)
	require.NoError(t, err)
	defer mgr.Close()

	for i := 0; i < 100; i++ {
		valid, _ := mgr.IsValidBaggageKey(expectedKey)
		if valid {
			break
		}
		time.Sleep(time.Millisecond)
	}
	valid, size := mgr.IsValidBaggageKey(expectedKey)
	require.True(t, valid)
	assert.EqualValues(t, expectedSize, size)

	valid, size = mgr.IsValidBaggageKey("bad-key")
	assert.False(t, valid)
	assert.EqualValues(t, 0, size)
}

type errCounterLogger struct {
	sync.RWMutex

	errCounter int64
}

func (l *errCounterLogger) Infof(msg string, args ...interface{}) {}

func (l *errCounterLogger) Error(msg string) {
	l.Lock()
	defer l.Unlock()
	l.errCounter++
}

func (l *errCounterLogger) getErrCounter() int64 {
	l.RLock()
	defer l.RUnlock()
	return l.errCounter
}

func TestConstructorError(t *testing.T) {
	_, err := NewRemoteRestrictionManager(
		"test",
		Options.RefreshInterval(10*time.Millisecond),
	)
	assert.Error(t, err)
}

func TestPollManagerError(t *testing.T) {
	logger := &errCounterLogger{}

	m := &RemoteRestrictionManager{
		serviceName: "test",
		thriftProxy: &httpRestrictionProxy{serverURL: ""},
		stopPoll:    make(chan struct{}),
		options: options{
			logger:          logger,
			metrics:         jaeger.NewNullMetrics(),
			refreshInterval: time.Millisecond,
		},
	}
	m.pollStopped.Add(1)
	defer m.Close()
	go m.pollManager()

	for i := 0; i < 100; i++ {
		if logger.getErrCounter() > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	assert.True(t, logger.getErrCounter() > 0)
}
