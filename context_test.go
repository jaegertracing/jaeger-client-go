package jaeger

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"
)

func TestContextFromString(t *testing.T) {
	var err error
	_, err = ContextFromString("")
	assert.Error(t, err)
	_, err = ContextFromString("abcd")
	assert.Error(t, err)
	_, err = ContextFromString("x:1:1:1")
	assert.Error(t, err)
	_, err = ContextFromString("1:x:1:1")
	assert.Error(t, err)
	_, err = ContextFromString("1:1:x:1")
	assert.Error(t, err)
	_, err = ContextFromString("1:1:1:x")
	assert.Error(t, err)
	_, err = ContextFromString("1:1:1:x")
	assert.Error(t, err)
	ctx, err := ContextFromString("1:1:1:1")
	assert.NoError(t, err)
	assert.EqualValues(t, 1, ctx.traceID)
	assert.EqualValues(t, 1, ctx.spanID)
	assert.EqualValues(t, 1, ctx.parentID)
	assert.EqualValues(t, 1, ctx.flags)
	ctx = NewSpanContext(1, 1, 1, true, nil)
	assert.EqualValues(t, 1, ctx.traceID)
	assert.EqualValues(t, 1, ctx.spanID)
	assert.EqualValues(t, 1, ctx.parentID)
	assert.EqualValues(t, 1, ctx.flags)
}

func TestSpanContext_WithBaggageItem(t *testing.T) {
	var ctx SpanContext
	ctx = ctx.WithBaggageItem("some-KEY", "Some-Value")
	assert.Equal(t, map[string]string{"some-KEY": "Some-Value"}, ctx.baggage)
	ctx = ctx.WithBaggageItem("some-KEY", "Some-Other-Value")
	assert.Equal(t, map[string]string{"some-KEY": "Some-Other-Value"}, ctx.baggage)
}

func TestSpanContext_SampledDebug(t *testing.T) {
	ctx, err := ContextFromString("1:1:1:1")
	require.NoError(t, err)
	assert.True(t, ctx.IsSampled())
	assert.False(t, ctx.IsDebug())

	ctx, err = ContextFromString("1:1:1:3")
	require.NoError(t, err)
	assert.True(t, ctx.IsSampled())
	assert.True(t, ctx.IsDebug())

	ctx, err = ContextFromString("1:1:1:0")
	require.NoError(t, err)
	assert.False(t, ctx.IsSampled())
	assert.False(t, ctx.IsDebug())
}
