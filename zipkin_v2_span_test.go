// Copyright (c) 2018 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package jaeger

import (
	"encoding/binary"
	"encoding/json"
	"testing"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	zipkinModel "github.com/openzipkin/zipkin-go/model"
	"github.com/stretchr/testify/assert"
	"github.com/uber/jaeger-client-go/utils"
)

// Creates a simple Zipkin span from a Jaeger span.  The mutator func is
// provided by the test function to modify the generated Jaeger span before it
// is finished and converted to Zipkin's span type.
func createTestZipkinV2Span(mutator func(*Span)) *zipkinModel.SpanModel {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()

	sp := tracer.StartSpan("s1").(*Span)
	mutator(sp)
	sp.Finish()

	return BuildZipkinV2Span(sp)
}

func TestZipkinV2TagConversion(t *testing.T) {
	zSpan := createTestZipkinV2Span(func(sp *Span) {
		sp.SetTag("a", 1)
		sp.SetTag("b", "test")
	})

	// There are two built-in tags: "sampler.type" and "sampler.param"
	assert.Len(t, zSpan.Tags, 4)

	// Converts ints to strings
	assert.Equal(t, "1", zSpan.Tags["a"])
	assert.Equal(t, "test", zSpan.Tags["b"])
}

func TestZipkinV2AnnotationConversion(t *testing.T) {
	zSpan := createTestZipkinV2Span(func(sp *Span) {
		sp.LogEvent("something happened")
		sp.LogKV("event", "unknown", "error", "none")
	})

	assert.Len(t, zSpan.Annotations, 2)

	assert.Equal(t, "something happened", zSpan.Annotations[0].Value)

	var fields map[string]string
	if err := json.Unmarshal([]byte(zSpan.Annotations[1].Value), &fields); err != nil {
		assert.FailNow(t, "annotation was not json", err.Error())
	}

	assert.Equal(t, "unknown", fields["event"])
	assert.Equal(t, "none", fields["error"])
}

func TestZipkinV2KindConversion(t *testing.T) {
	tests := []struct {
		jaegerKind ext.SpanKindEnum
		zipkinKind zipkinModel.Kind
	}{
		{
			jaegerKind: ext.SpanKindRPCClientEnum,
			zipkinKind: zipkinModel.Client,
		},
		{
			jaegerKind: ext.SpanKindRPCServerEnum,
			zipkinKind: zipkinModel.Server,
		},
		{
			jaegerKind: ext.SpanKindProducerEnum,
			zipkinKind: zipkinModel.Producer,
		},
		{
			jaegerKind: ext.SpanKindConsumerEnum,
			zipkinKind: zipkinModel.Consumer,
		},
	}
	for _, test := range tests {
		zSpan := createTestZipkinV2Span(func(sp *Span) {
			sp.SetTag(string(ext.SpanKind), test.jaegerKind)
		})

		assert.Equal(t, test.zipkinKind, zSpan.Kind)
	}
}

func TestZipkinV2LocalEndpointConversion(t *testing.T) {
	zSpan := createTestZipkinV2Span(func(sp *Span) {})

	hostIP, err := utils.HostIP()
	if err != nil {
		assert.FailNow(t, "Could not determine local ip: ", err.Error())
	}

	assert.Equal(t, hostIP.String(), zSpan.LocalEndpoint.IPv4.String())
}

func TestZipkinV2TraceIDConversion(t *testing.T) {
	var jaegerTraceID TraceID

	zSpan := createTestZipkinV2Span(func(sp *Span) {
		jaegerTraceID = sp.context.TraceID()
	})

	assert.Equal(t, jaegerTraceID.Low, zSpan.TraceID.Low)
	assert.Equal(t, jaegerTraceID.High, zSpan.TraceID.High)
}

func TestZipkinV2RemoteEndpointConversion(t *testing.T) {
	for _, tc := range []struct {
		service interface{}
		port    interface{}
		ipv4    interface{}
	}{
		{
			service: "peer",
			port:    uint16(80),
			ipv4:    uint32(2130706433),
		},
		{
			service: "peer",
			port:    "80",
			ipv4:    "127.0.0.1",
		},
		{
			service: "peer",
			port:    int(80),
			ipv4:    int32(2130706433),
		},
	} {

		zSpan := createTestZipkinV2Span(func(sp *Span) {
			sp.SetTag(string(ext.PeerService), tc.service)
			sp.SetTag(string(ext.PeerPort), tc.port)
			sp.SetTag(string(ext.PeerHostIPv4), tc.ipv4)
		})

		assert.NotNil(t, zSpan.RemoteEndpoint)
		assert.Equal(t, uint32(2130706433), binary.BigEndian.Uint32(zSpan.RemoteEndpoint.IPv4))
		assert.Equal(t, uint16(80), zSpan.RemoteEndpoint.Port)
		assert.Equal(t, "peer", zSpan.RemoteEndpoint.ServiceName)
	}
}

func TestZipkinV2ParentSpanID(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()

	sp := tracer.StartSpan("s1")
	sp2 := tracer.StartSpan("s2", opentracing.ChildOf(sp.Context())).(*Span)
	sp2.Finish()
	sp.Finish()

	zSpan := BuildZipkinV2Span(sp2)

	assert.NotNil(t, zSpan.ParentID)
	assert.Equal(t, *zSpan.ParentID, zipkinModel.ID(sp.(*Span).context.SpanID()))
}
