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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go/testutils"
	"github.com/uber/jaeger-client-go/thrift-gen/baggage"
)

const (
	service      = "svc"
	expectedKey  = "key"
	expectedSize = int32(10)
)

func startMockAgent(t *testing.T) *testutils.MockAgent {
	agent, err := testutils.StartMockAgent()
	require.NoError(t, err)

	agent.AddBaggageRestrictions(service, []*baggage.BaggageRestriction{
		{BaggageKey: expectedKey, MaxValueLength: expectedSize},
	})
	return agent
}

func restartManager(mgr *RemoteRestrictionManager, agent *testutils.MockAgent) {
	mgr.thriftProxy = &httpRestrictionProxy{serverURL: "http://" + agent.ServerAddr() + "/baggageRestrictions"}
	mgr.stopPoll = make(chan struct{})
	mgr.pollStopped.Add(1)
}

func TestNewRemoteRestrictionManager(t *testing.T) {
	agent := startMockAgent(t)
	defer agent.Close()

	mgr := NewRemoteRestrictionManager(
		service,
		Options.RefreshInterval(10*time.Millisecond),
		Options.ServerURL("http://"+agent.ServerAddr()+"/baggageRestrictions"),
	)
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

func TestFailClosed(t *testing.T) {
	mgr := NewRemoteRestrictionManager(
		service,
		Options.ServerURL(""),
		Options.FailClosed(true),
		Options.RefreshInterval(time.Millisecond),
	)

	valid, _ := mgr.IsValidBaggageKey(expectedKey)
	assert.False(t, valid)
	mgr.Close()

	agent := startMockAgent(t)
	defer agent.Close()

	restartManager(mgr, agent)
	go mgr.pollManager()
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
}

func TestFailOpen(t *testing.T) {
	mgr := NewRemoteRestrictionManager(
		service,
		Options.ServerURL(""),
		Options.RefreshInterval(time.Millisecond),
	)

	valid, size := mgr.IsValidBaggageKey(expectedKey)
	assert.True(t, valid)
	assert.Equal(t, 2048, size)
	mgr.Close()

	agent := startMockAgent(t)
	defer agent.Close()

	restartManager(mgr, agent)
	go mgr.pollManager()
	defer mgr.Close()

	for i := 0; i < 100; i++ {
		valid, size := mgr.IsValidBaggageKey(expectedKey)
		if valid && size == int(expectedSize) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	valid, size = mgr.IsValidBaggageKey(expectedKey)
	require.True(t, valid)
	assert.EqualValues(t, expectedSize, size)
}
