# Zipkin compatibility features

## `NewZipkinB3HTTPHeaderPropagator()`

Adds support for injecting and extracting Zipkin B3 Propagation HTTP headers,
for use with other Zipkin collectors.

```go

// ...
import (
  "github.com/uber/jaeger-client-go/zipkin"
)

func main() {
	// ...
	zipkinPropagator := zipkin.NewZipkinB3HTTPHeaderPropagator()
	injector := jaeger.TracerOptions.Injector(opentracing.HTTPHeaders, zipkinPropagator)
	extractor := jaeger.TracerOptions.Extractor(opentracing.HTTPHeaders, zipkinPropagator)

	// create Jaeger tracer
	tracer, closer := jaeger.NewTracer(
		"myService",
		mySampler, // as usual
		myReporter // as usual
		injector,
		extractor,
	)
}
```
