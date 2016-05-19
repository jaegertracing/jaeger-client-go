package utils

import "time"

// RateLimiter is a filter used to check if a message that is worth itemCost units is within the rate limits.
type RateLimiter interface {
	CheckCredit(itemCost float64) bool
}

type rateLimiter struct {
	creditsPerSecond float64
	balance          float64
	lastTick         time.Time

	timeNow func() time.Time
}

// NewRateLimiter creates a new rate limiter based on leaky bucket algorithm, formulated in terms of a
// credits balance that is replenished every time CheckCredit() method is called (tick) by the amount proportional
// to the time elapsed since the last tick, up to max of creditsPerSecond. A call to CheckCredit() takes a cost
// of an item we want to pay with the balance. If the balance exceeds the cost of the item, the item is "purchased"
// and the balance reduced, indicated by returned value of true. Otherwise the balance is unchanged and return false.
//
// This can be used to limit a rate of messages emitted by a service by instantiating the Rate Limiter with the
// max number of messages a service is allowed to emit per second, and calling CheckCredit(1.0) for each message
// to determine if the message is within the rate limit.
//
// It can also be used to limit the rate of traffic in bytes, by setting creditsPerSecond to desired throughput
// as bytes/second, and calling CheckCredit() with the actual message size.
func NewRateLimiter(creditsPerSecond float64) RateLimiter {
	return &rateLimiter{
		creditsPerSecond: creditsPerSecond,
		balance:          creditsPerSecond,
		lastTick:         time.Now(),
		timeNow:          time.Now}
}

func (b *rateLimiter) CheckCredit(itemCost float64) bool {
	// calculate how much time passed since the last tick, and update current tick
	currentTime := b.timeNow()
	elapsedTime := currentTime.Sub(b.lastTick)
	b.lastTick = currentTime
	// calculate how much credit have we accumulated since the last tick
	b.balance += elapsedTime.Seconds() * b.creditsPerSecond
	if b.balance > b.creditsPerSecond {
		b.balance = b.creditsPerSecond
	}
	// if we have enough credits to pay for current item, then reduce balance and allow
	if b.balance >= itemCost {
		b.balance -= itemCost
		return true
	}
	return false
}
