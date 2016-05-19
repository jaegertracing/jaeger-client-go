package testutils_test

import (
	"testing"
	"time"

	tu "github.com/uber/jaeger-client-go/testutils"
	"github.com/uber/jaeger-client-go/thrift/gen/sampling"
	"github.com/uber/jaeger-client-go/thrift/gen/tcollector"
	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/tchannel-go/thrift"
)

func withTCollector(t *testing.T, fn func(collector *tu.MockTCollector, ctx thrift.Context)) {
	collector, err := tu.StartMockTCollector()
	require.NoError(t, err)
	defer collector.Close()

	time.Sleep(10 * time.Millisecond) // give the server a chance to start

	ctx, ctxCancel := thrift.NewContext(time.Second)
	defer ctxCancel()

	fn(collector, ctx)
}

func withSamplingClient(t *testing.T, fn func(collector *tu.MockTCollector, ctx thrift.Context, client sampling.TChanSamplingManager)) {
	withTCollector(t, func(collector *tu.MockTCollector, ctx thrift.Context) {
		thriftClient := thrift.NewClient(collector.Channel, "tcollector", nil)
		client := sampling.NewTChanSamplingManagerClient(thriftClient)

		fn(collector, ctx, client)
	})
}

func withTCollectorClient(t *testing.T, fn func(collector *tu.MockTCollector, ctx thrift.Context, client tcollector.TChanTCollector)) {
	withTCollector(t, func(collector *tu.MockTCollector, ctx thrift.Context) {
		thriftClient := thrift.NewClient(collector.Channel, "tcollector", nil)
		client := tcollector.NewTChanTCollectorClient(thriftClient)

		fn(collector, ctx, client)
	})
}

func withZipkinClient(t *testing.T, fn func(collector *tu.MockTCollector, ctx thrift.Context, client zipkincore.TChanZipkinCollector)) {
	withTCollector(t, func(collector *tu.MockTCollector, ctx thrift.Context) {
		thriftClient := thrift.NewClient(collector.Channel, "tcollector", nil)
		client := zipkincore.NewTChanZipkinCollectorClient(thriftClient)

		fn(collector, ctx, client)
	})
}

func TestMockTCollector(t *testing.T) {
	withSamplingClient(t, func(collector *tu.MockTCollector, ctx thrift.Context, client sampling.TChanSamplingManager) {
		s, err := client.GetSamplingStrategy(ctx, "default-service")
		require.NoError(t, err)
		require.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, s.StrategyType)
		require.NotNil(t, s.ProbabilisticSampling)
		assert.Equal(t, 0.01, s.ProbabilisticSampling.SamplingRate)

		collector.AddSamplingStrategy("service1", &sampling.SamplingStrategyResponse{
			StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
			RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
				MaxTracesPerSecond: 10,
			}})

		s, err = client.GetSamplingStrategy(ctx, "service1")
		require.NoError(t, err)
		require.Equal(t, sampling.SamplingStrategyType_RATE_LIMITING, s.StrategyType)
		require.NotNil(t, s.RateLimitingSampling)
		assert.EqualValues(t, 10, s.RateLimitingSampling.MaxTracesPerSecond)
	})

	withTCollectorClient(t, func(collector *tu.MockTCollector, ctx thrift.Context, client tcollector.TChanTCollector) {
		span := &tcollector.Span{
			TraceId:           []byte{0, 0, 0, 0, 0, 0, 0, 1},
			Host:              &tcollector.Endpoint{Ipv4: 0, Port: 0, ServiceName: "service1"},
			Name:              "service2",
			ID:                []byte{0, 0, 0, 0, 0, 0, 0, 1},
			ParentId:          []byte{0, 0, 0, 0, 0, 0, 0, 0},
			Annotations:       make([]*tcollector.Annotation, 0),
			BinaryAnnotations: make([]*tcollector.BinaryAnnotation, 0),
		}
		_, err := client.Submit(ctx, span)
		require.NoError(t, err)
		_, err = client.SubmitBatch(ctx, []*tcollector.Span{span, span})
		require.NoError(t, err)
		spans := collector.GetTChannelSpans()
		assert.Equal(t, 3, len(spans))
		for _, s := range spans {
			assert.Equal(t, "service2", s.Name)
		}
	})

	withZipkinClient(t, func(collector *tu.MockTCollector, ctx thrift.Context, client zipkincore.TChanZipkinCollector) {
		span := &zipkincore.Span{Name: "service3"}
		_, err := client.SubmitZipkinBatch(ctx, []*zipkincore.Span{span})
		require.NoError(t, err)
		spans := collector.GetZipkinSpans()
		require.Equal(t, 1, len(spans))
		assert.Equal(t, "service3", spans[0].Name)
	})

	collector := &tu.MockTCollector{}
	_, err := collector.GetSamplingStrategy(nil, "service1")
	assert.Error(t, err)
}
