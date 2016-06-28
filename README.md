[![GoDoc][doc-img]][doc] [![Build Status][ci-img]][ci] [![Coverage Status][cov-img]][cov]

# jaeger-client-go

This is a client side library that implements an
[OpenTracing](http://opentracing.io) Tracer, with Zipkin-compatible
data model.

## Initialization

```go
import (
    "github.com/opentracing/opentracing-go"
    "github.com/uber/jaeger-client-go/config"
)

type AppConfig struct {
    ...
    Tracing config.Configuration
    ...
}

func main() {
    cfg := loadAppConfig() // e.g. from a yaml file

    tracer, closer, err := cfg.Tracing.New("your-service-name", nil)
    // check err
    defer closer()

    opentracing.InitGlobalTracer(tracer)
    ...
}
```

### Metrics & Monitoring

The tracer emits a number of different metrics, defined in
[metrics.go](metrics.go). The monitoring backend is expected to support
tag-based metric names, e.g. instead of `statsd`-style string names
like `counters.my-service.jaeger.spans.started.sampled`, the metrics
are defined by a short name and a collection of key/value tags, for
example: `name:traces, state:started, sampled:true`.

The monitoring backend is represented by the
[StatsReporter](stats_reporter.go) interface. An implementation
of that interface should be passed to the `New` method during
tracer initialization:

```go
    stats := // create StatsReporter implementation
    tracer := config.Tracing.New("your-service-name", stats)
```

By default, a no-op `NullStatsReporter` is used.

### Logging

The tracer can be configured with an optional logger, which will be
used to log communication errors, or log spans if a logging reporter
option is specified in the configuration. The logging API is abstracted
by the [Logger](logger.go) interface. A logger instance implementing
this interface can be set on the `Config` object before calling the
`New` method.

## Instrumentation for Tracing

Since this tracer is fully compliant with OpenTracing API 1.0,
all code instrumentation should only use the API itself, as described
in [opentracing-go]
(https://github.com/opentracing/opentracing-go) documentation.

## Features

### Reporters

A "reporter" is a component receives the finished spans and reports
them to somewhere. Under normal circumstances, the Tracer
should use the default `RemoteReporter`, which sends the spans out of
process via configurable "transport". For testing purposes, one can
use an `InMemoryReporter` that accumulates spans in a buffer and
allows to retrieve them for later verification. Also available are
`NullReporter`, a no-op reporter that does nothing, a `LoggingReporter`
which logs all finished spans using their `String()` method, and a
`CompositeReporter` that can be used to combine more than one reporter
into one, e.g. to attach a logging reporter to the main remote reporter.

### Span Reporting Transports

The remote reporter uses "transports" to actually send the spans out
of process. Currently two supported transports are Thrift over UDP
and Thrift over TChannel. More transports will be added in the future.

The only data format currently used is Zipkin Thrift 1.x span format,
which allows easy integration of the tracer with Zipkin backend.

### Sampling

The tracer does not record all spans, but only those that have the
sampling bit set in the `flags`. When a new trace is started and a new
unique ID is generated, a sampling decision is made whether this trace
should be sampled. The sampling decision is propagated to all downstream
calls via the `flags` field of the trace context. The following samplers
are available:
  1. `RemotelyControlledSampler` uses one of the other simpler samplers
     and periodically updates it by polling an external server. This
     allows dynamic control of the sampling strategies.
  1. `ConstSampler` always makes the same sampling decision for all
     trace IDs. it can be configured to either sample all traces, or
     to sample none.
  1. `ProbabilisticSampler` uses a fixed sampling rate as a probability
     for a given trace to be sampled. The actual decision is made by
     comparing the trace ID with a random number multiplied by the
     sampling rate.
  1. `RateLimitingSampler` can be used to allow only a certain fixed
     number of traces to be sampled per second.


[doc-img]: https://godoc.org/github.com/uber/jaeger-client-go?status.svg
[doc]: https://godoc.org/github.com/uber/jaeger-client-go
[ci-img]: https://travis-ci.org/uber/jaeger-client-go.svg?branch=master
[ci]: https://travis-ci.org/uber/jaeger-client-go
[cov-img]: https://coveralls.io/repos/uber/jaeger-client-go/badge.svg?branch=master&service=github
[cov]: https://coveralls.io/github/uber/jaeger-client-go?branch=master
