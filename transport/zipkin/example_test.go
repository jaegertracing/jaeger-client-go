package zipkin_test

import (
	"log"

	"github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	"github.com/uber/jaeger-client-go/transport/zipkin"
)

func ExampleNewHTTPTransport() {
	// assume this is your main()

	transport, err := zipkin.NewHTTPTransport(
		"http://localhost:9411/api/v1/spans",
		zipkin.HTTPBatchSize(10),
		zipkin.HTTPLogger(jaeger.StdLogger),
	)
	if err != nil {
		log.Fatalf("Cannot initialize Zipkin HTTP transport: %v", err)
	}
	tracer, closer := jaeger.NewTracer(
		"my-service-name",
		jaeger.NewConstSampler(true),
		jaeger.NewRemoteReporter(transport, nil),
	)
	defer closer.Close()
	opentracing.InitGlobalTracer(tracer)

	// initialize servers
}
