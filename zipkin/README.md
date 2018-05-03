# Zipkin compatibility features

## `NewZipkinB3HTTPHeaderPropagator()`

Adds support for injecting and extracting Zipkin B3 Propagation HTTP headers,
for use with other Zipkin collectors.

```go

// ...
import (
  opentracing "github.com/opentracing/opentracing-go"
  jaeger "github.com/uber/jaeger-client-go"
  "github.com/uber/jaeger-client-go/zipkin"
)

func main() {
	// ...
	zipkinPropagator := zipkin.NewZipkinB3HTTPHeaderPropagator()
	injector := jaeger.TracerOptions.Injector(opentracing.HTTPHeaders, zipkinPropagator)
	extractor := jaeger.TracerOptions.Extractor(opentracing.HTTPHeaders, zipkinPropagator)
	
	// Zipkin shares span ID between client and server spans; it must be enabled via the following option.
	zipkinSharedRPCSpan := jaeger.TracerOptions.ZipkinSharedRPCSpan(true)

	// create Jaeger tracer
	tracer, closer := jaeger.NewTracer(
		"myService",
		mySampler, // as usual
		myReporter // as usual
		injector,
		extractor,
		zipkinSharedRPCSpan,
	)
}
```

If you'd like to follow the official guides from https://godoc.org/github.com/uber/jaeger-client-go/config#example-Configuration-InitGlobalTracer-Testing, here is an example.

```go
import (
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegerClientConfig "github.com/uber/jaeger-client-go/config"
	"github.com/uber/jaeger-client-go/zipkin"
)

// Create jaeger config
cfg := jaegerClientConfig.Configuration{
  Sampler: &jaegerClientConfig.SamplerConfig{
	Type:  "const",
	Param: 1,
  },
  Reporter: &jaegerClientConfig.ReporterConfig{
	LogSpans:            true,
	BufferFlushInterval: 1 * time.Second,
  },
}

// Zipkin shares span ID between client and server spans; it must be enabled via the following option.
zipkinPropagator := zipkin.NewZipkinB3HTTPHeaderPropagator()

//Initialize tracer
closer, err := cfg.InitGlobalTracer(
  serviceName,
  jaegerClientConfig.Logger(jaeger.StdLogger),
  jaegerClientConfig.Injector(opentracing.HTTPHeaders, zipkinPropagator),
  jaegerClientConfig.Extractor(opentracing.HTTPHeaders, zipkinPropagator),
  jaegerClientConfig.ZipkinSharedRPCSpan(true),
)
```
