package jaeger

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(2.0)
	// stop time
	ts := time.Now()
	limiter.(*rateLimiter).lastTick = ts
	limiter.(*rateLimiter).timeNow = func() time.Time {
		return ts
	}
	assert.True(t, limiter.CheckCredit(1.0))
	assert.True(t, limiter.CheckCredit(1.0))
	assert.False(t, limiter.CheckCredit(1.0))
	// move time 250ms forward, not enough credits to pay for 1.0 item
	limiter.(*rateLimiter).timeNow = func() time.Time {
		return ts.Add(time.Second / 4)
	}
	assert.False(t, limiter.CheckCredit(1.0))
	// move time 500ms forward, now enough credits to pay for 1.0 item
	limiter.(*rateLimiter).timeNow = func() time.Time {
		return ts.Add(time.Second/4 + time.Second/2)
	}
	assert.True(t, limiter.CheckCredit(1.0))
	assert.False(t, limiter.CheckCredit(1.0))
	// move time 5s forward, enough to accumulate credits for 10 messages, but it should still be capped at 2
	limiter.(*rateLimiter).lastTick = ts
	limiter.(*rateLimiter).timeNow = func() time.Time {
		return ts.Add(5 * time.Second)
	}
	assert.True(t, limiter.CheckCredit(1.0))
	assert.True(t, limiter.CheckCredit(1.0))
	assert.False(t, limiter.CheckCredit(1.0))
	assert.False(t, limiter.CheckCredit(1.0))
	assert.False(t, limiter.CheckCredit(1.0))
}
