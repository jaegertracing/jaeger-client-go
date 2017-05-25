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
	"sync"
	"time"
)

// OverflowChecker is a filter that checks if an event has occurred more than a specified number of times.
type OverflowChecker interface {
	CheckOverflow() bool
}

type overflowChecker struct {
	sync.Mutex

	lastTick   time.Time
	balance    int64
	maxBalance int64
	duration   time.Duration

	timeNow func() time.Time
}

// NewOverflowChecker creates a new overflow checker formulated in terms of a maxBalance and duration.
// The first call to CheckOverflow() creates a new balance with a TTL of duration. Every subsequent
// CheckOverflow() call increments the balance unless the balance has expired. If the balance expired,
// a new balance will be initialized with a TTL of duration.
//
// Once the balance is greater than maxBalance, an overflow has occurred and the function will return
// true. Otherwise it returns false. Once an overflow occurs, a new balance is initialized with a TTL
// of duration.
func NewOverflowChecker(maxBalance int64, duration time.Duration) OverflowChecker {
	return &overflowChecker{
		maxBalance: maxBalance,
		duration:   duration,
		lastTick:   time.Now(),
		timeNow:    time.Now,
	}
}

func (s *overflowChecker) CheckOverflow() bool {
	s.Lock()
	defer s.Unlock()
	currentTime := s.timeNow()
	elapsedTime := currentTime.Sub(s.lastTick)
	if elapsedTime > s.duration {
		// Start new balance
		s.balance = 1
		s.lastTick = currentTime
		return false
	}
	s.balance++
	if s.balance > s.maxBalance {
		// Start new balance
		s.balance = 1
		s.lastTick = currentTime
		return true
	}
	return false
}
