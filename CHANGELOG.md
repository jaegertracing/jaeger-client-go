Changes by Version
==================

2.2.0 (Unreleased)
------------------

- Introduce Observer and SpanObserver


2.1.2 (2016-02-27)
-------------------

- Fix leaky bucket bug (https://github.com/uber/jaeger-client-go/pull/99)
- Fix zap logger Infof (https://github.com/uber/jaeger-client-go/pull/100)
- Add tracer initialization godoc examples


2.1.1 (2016-02-21)
-------------------

- Fix inefficient usage of zap.Logger


2.1.0 (2016-02-17)
-------------------

- Add adapter for zap.Logger (https://github.com/uber-go/zap)
- Move logging API to ./log/ package


2.0.0 (2016-02-08)
-------------------

- Support Adaptive Sampling
- Support 128bit Trace IDs
- Change trace/span IDs from uint64 to strong types TraceID and SpanID
- Add Zipkin HTTP B3 Propagation format support #72
- Rip out existing metrics and use github.com/uber/jaeger-lib/metrics
- Change API for tracer, reporter, sampler initialization


1.6.0 (2016-10-14)
-------------------

- Add Zipkin HTTP transport
- Support external baggage via jaeger-baggage header
- Unpin Thrift version, keep to master


1.5.1 (2016-09-27)
-------------------

- Relax dependency on opentracing to ^1


1.5.0 (2016-09-27)
-------------------

- Upgrade to opentracing-go 1.0
- Support KV logging for Spans


1.4.0 (2016-09-14)
-------------------

- Support debug traces via HTTP header "jaeger-debug-id"
