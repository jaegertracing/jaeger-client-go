package testutils

import (
	"fmt"
	"testing"
	"time"

	"github.com/uber/jaeger-client-go/thrift/gen/sampling"
	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"
	"github.com/uber/jaeger-client-go/utils"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockAgentSpanServer(t *testing.T) {
	mockAgent, err := StartMockAgent()
	require.NoError(t, err)
	defer mockAgent.Close()

	client, err := mockAgent.SpanServerClient()
	require.NoError(t, err)

	for i := 1; i < 5; i++ {
		spans := make([]*zipkincore.Span, i, i)
		for j := 0; j < i; j++ {
			spans[j] = zipkincore.NewSpan()
			spans[j].Name = fmt.Sprintf("span-%d", j)
		}

		err = client.EmitZipkinBatch(spans)
		assert.NoError(t, err)
		time.Sleep(5 * time.Millisecond)

		spans = mockAgent.GetZipkinSpans()
		require.Equal(t, i, len(spans))
		for j := 0; j < i; j++ {
			assert.Equal(t, fmt.Sprintf("span-%d", j), spans[j].Name)
		}
		mockAgent.ResetZipkinSpans()
	}
}

func TestMockAgentSamplingManager(t *testing.T) {
	mockAgent, err := StartMockAgent()
	require.NoError(t, err)
	defer mockAgent.Close()

	err = utils.GetJSON("http://"+mockAgent.SamplingServerAddr()+"/", nil)
	require.Error(t, err, "no 'service' parameter")
	err = utils.GetJSON("http://"+mockAgent.SamplingServerAddr()+"/?service=a&service=b", nil)
	require.Error(t, err, "Too many 'service' parameters")

	var resp sampling.SamplingStrategyResponse
	err = utils.GetJSON("http://"+mockAgent.SamplingServerAddr()+"/?service=something", &resp)
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_PROBABILISTIC, resp.StrategyType)

	mockAgent.AddSamplingStrategy("service123", &sampling.SamplingStrategyResponse{
		StrategyType: sampling.SamplingStrategyType_RATE_LIMITING,
		RateLimitingSampling: &sampling.RateLimitingSamplingStrategy{
			MaxTracesPerSecond: 123,
		},
	})
	err = utils.GetJSON("http://"+mockAgent.SamplingServerAddr()+"/?service=service123", &resp)
	require.NoError(t, err)
	assert.Equal(t, sampling.SamplingStrategyType_RATE_LIMITING, resp.StrategyType)
	require.NotNil(t, resp.RateLimitingSampling)
	assert.EqualValues(t, 123, resp.RateLimitingSampling.MaxTracesPerSecond)
}
