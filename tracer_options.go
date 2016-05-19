package jaeger

import "time"

// TracerOption is a function that sets some option on the tracer
type TracerOption func(tracer *tracer)

// TracerOptions is a factory for all available TracerOption's
var TracerOptions tracerOptions

type tracerOptions struct{}

// Metrics creates a TracerOption that initializes Metrics on the tracer,
// which is used to emit statistics.
func (tracerOptions) Metrics(m *Metrics) TracerOption {
	return func(tracer *tracer) {
		tracer.metrics = *m
	}
}

// TimeNow creates a TracerOption that gives the tracer a function
// used to generate timestamps for spans.
func (tracerOptions) TimeNow(timeNow func() time.Time) TracerOption {
	return func(tracer *tracer) {
		tracer.timeNow = timeNow
	}
}

// RandomNumber creates a TracerOption that gives the tracer
// a thread-safe random number generator function for generating trace IDs.
func (tracerOptions) RandomNumber(randomNumber func() uint64) TracerOption {
	return func(tracer *tracer) {
		tracer.randomNumber = randomNumber
	}
}

// PoolSpans creates a TracerOption that tells the tracer whether it should use
// an object pool to minimize span allocations.
// This should be used with care, only if the service is not running any async tasks
// that can access parent spans after those spans have been finished.
func (tracerOptions) PoolSpans(poolSpans bool) TracerOption {
	return func(tracer *tracer) {
		tracer.poolSpans = poolSpans
	}
}

// HostIPv4 creates a TracerOption that identifies the current service/process.
// If not set, the factory method will obtain the current IP address.
func (tracerOptions) HostIPv4(hostIPv4 uint32) TracerOption {
	return func(tracer *tracer) {
		tracer.hostIPv4 = hostIPv4
	}
}
