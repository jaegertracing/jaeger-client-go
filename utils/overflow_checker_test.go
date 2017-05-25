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

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOverflowChecker(t *testing.T) {
	s := NewOverflowChecker(2, time.Second)
	// stop time
	ts := time.Now()
	s.(*overflowChecker).lastTick = ts
	s.(*overflowChecker).timeNow = func() time.Time {
		return ts
	}
	assert.False(t, s.CheckOverflow())
	assert.False(t, s.CheckOverflow())
	assert.True(t, s.CheckOverflow())

	assert.False(t, s.CheckOverflow())
	// move time 1.1s forward, the bucket should empty
	s.(*overflowChecker).timeNow = func() time.Time {
		return ts.Add(1100 * time.Millisecond)
	}
	assert.False(t, s.CheckOverflow())
	assert.False(t, s.CheckOverflow())
	assert.True(t, s.CheckOverflow())

	assert.False(t, s.CheckOverflow())
	// move time 200ms forward, not enough time for the bucket to empty
	s.(*overflowChecker).timeNow = func() time.Time {
		return ts.Add(1300 * time.Millisecond)
	}
	assert.True(t, s.CheckOverflow())
}
