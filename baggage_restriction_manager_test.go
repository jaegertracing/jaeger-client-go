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
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/uber/jaeger-client-go/thrift-gen/baggage"
)

func TestNewBaggageRestrictionManager(t *testing.T) {
	agent, _, _ := initAgent(t)
	defer agent.Close()

	service := "svc"
	expectedKey := "key"
	expectedSize := int32(10)

	agent.AddBaggageRestrictions(service, []*baggage.BaggageRestriction{
		{BaggageKey: expectedKey, MaxValueLength: expectedSize},
	})

	mgr := NewBaggageRestrictionManager(
		service,
		BaggageRestrictionManagerOptions.RefreshInterval(10*time.Millisecond),
		BaggageRestrictionManagerOptions.BaggageRestrictionManagerServerURL("http://"+agent.AgentServerAddr()),
	)
	defer mgr.(*baggageRestrictionManager).Close()

	for i := 0; i < 100; i++ {
		valid, _ := mgr.IsValidBaggageKey(expectedKey)
		if valid {
			break
		}
		time.Sleep(time.Millisecond)
	}
	valid, size := mgr.IsValidBaggageKey(expectedKey)
	require.True(t, valid)
	require.Equal(t, expectedSize, size)
}
