package rpcmetrics

import (
	"testing"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-lib/metrics"
	"github.com/uber/jaeger-lib/metrics/testutils"

	"github.com/opentracing/opentracing-go/ext"
	jaeger "github.com/uber/jaeger-client-go"
)

type testTracer struct {
	metrics *metrics.LocalFactory
	tracer  opentracing.Tracer
}

func withTestTracer(runTest func(tt *testTracer)) {
	sampler := jaeger.NewConstSampler(true)
	reporter := jaeger.NewInMemoryReporter()
	metrics := metrics.NewLocalFactory(time.Minute)
	observer := NewObserver(metrics, DefaultNameNormalizer)
	tracer, closer := jaeger.NewTracer(
		"test",
		sampler,
		reporter,
		jaeger.TracerOptions.Observer(observer))
	defer closer.Close()
	runTest(&testTracer{
		metrics: metrics,
		tracer:  tracer,
	})
}

func TestNonRPCSpan(t *testing.T) {
	withTestTracer(func(testTracer *testTracer) {
		span := testTracer.tracer.StartSpan("test")
		span.Finish()

		c, _ := testTracer.metrics.Snapshot()
		assert.Len(t, c, 0)

		testutils.AssertCounterMetrics(t,
			testTracer.metrics,
			testutils.ExpectedMetric{},
		)
	})
}

var endpointTag = map[string]string{"endpoint": "get-user"}

func TestRPCServerSpan(t *testing.T) {
	withTestTracer(func(testTracer *testTracer) {
		ts := time.Now()
		span := testTracer.tracer.StartSpan(
			"get-user",
			ext.SpanKindRPCServer,
			opentracing.StartTime(ts),
		)
		span.FinishWithOptions(opentracing.FinishOptions{
			FinishTime: ts.Add(50 * time.Millisecond),
		})

		_, g := testTracer.metrics.Snapshot()

		testutils.AssertCounterMetrics(t,
			testTracer.metrics,
			testutils.ExpectedMetric{Name: "requests", Tags: endpointTag, Value: 1},
			testutils.ExpectedMetric{Name: "success", Tags: endpointTag, Value: 1},
		)
		// TODO something wrong with string generation, .P99 should not be appended to the tag
		assert.EqualValues(t, 51, g["request_latency_ms|endpoint=get-user.P99"])
	})
}
