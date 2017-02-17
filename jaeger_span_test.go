// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package jaeger

import (
	"fmt"
	"testing"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/log"
	"github.com/stretchr/testify/assert"
	j "github.com/uber/jaeger-client-go/thrift-gen/jaeger"
)

func TestBuildJaegerSpan(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()

	sp1 := tracer.StartSpan("sp1").(*span)
	sp2 := tracer.StartSpan("sp2", opentracing.ChildOf(sp1.Context())).(*span)
	sp2.Finish()
	sp1.Finish()

	jaegerSpan1 := buildJaegerSpan(sp1)
	jaegerSpan2 := buildJaegerSpan(sp2)
	assert.Equal(t, "sp1", jaegerSpan1.OperationName)
	assert.Equal(t, "sp2", jaegerSpan2.OperationName)
	assert.EqualValues(t, 0, jaegerSpan1.ParentSpanId)
	assert.Equal(t, jaegerSpan1.SpanId, jaegerSpan2.ParentSpanId)
	assert.Len(t, jaegerSpan1.Tags, 4)
	tag := findTag(jaegerSpan1, SamplerTypeTagKey)
	assert.Equal(t, SamplerTypeConst, *tag.VStr)
}

func TestBuildLogs(t *testing.T) {
	tracer, closer := NewTracer("DOOP",
		NewConstSampler(true),
		NewNullReporter())
	defer closer.Close()
	root := tracer.StartSpan("s1")

	//someTime := time.Now().Add(-time.Minute)
	//someTimeInt64 := utils.TimeToMicrosecondsSinceEpochInt64(someTime)

	someString := "happened"
	someBool := true
	someLong := 123
	someDouble := 123

	fields := func(fields ...log.Field) []log.Field {
		return fields
	}
	tests := []struct {
		fields            []log.Field
		logFunc           func(sp opentracing.Span)
		expected          []*j.Tag
		expectedTimestamp int64
		disableSampling   bool
	}{
		{fields: fields(log.String("event", "happened")), expected: []*j.Tag{{Key: "event", VType: j.TagType_STRING, VStr: &someString}}},
		{fields: fields(log.String("something", "happened")), expected: []*j.Tag{{Key: "something", VType: j.TagType_STRING, VStr: &someString}}},
		{fields: fields(log.Bool("something", true)), expected: []*j.Tag{{Key: "something", VType: j.TagType_BOOL, VStr: &someBool}}},
		{fields: fields(log.Int("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_LONG, VStr: &someLong}}},
		{fields: fields(log.Int32("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_LONG, VStr: &someLong}}},
		{fields: fields(log.Int64("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_LONG, VStr: &someLong}}},
		{fields: fields(log.Uint32("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_LONG, VStr: &someLong}}},
		{fields: fields(log.Uint64("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_LONG, VStr: &someLong}}},
		{fields: fields(log.Float32("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_DOUBLE, VStr: &someDouble}}},
		{fields: fields(log.Float64("something", 123)), expected: []*j.Tag{{Key: "something", VType: j.TagType_DOUBLE, VStr: &someDouble}}},
		//{fields: fields(log.Error(errors.New("drugs are baaad, m-k"))),
		//	expected: `{"error":"drugs are baaad, m-k"}`},
		//{fields: fields(log.Object("something", 123)), expected: `{"something":"123"}`},
	}
	for i, test := range tests {
		testName := fmt.Sprintf("test-%02d", i)
		sp := tracer.StartSpan(testName, opentracing.ChildOf(root.Context()))
		//if test.disableSampling {
		//	ext.SamplingPriority.Set(sp, 0)
		//}
		if test.logFunc != nil {
			test.logFunc(sp)
		} else if len(test.fields) > 0 {
			sp.LogFields(test.fields...)
		}
		thriftSpan := buildJaegerSpan(sp.(*span))
		//if test.disableSampling {
		//	assert.Equal(t, 0, len(thriftSpan.Annotations), testName)
		//	continue
		//}
		assert.Equal(t, 1, len(thriftSpan.Logs), testName)
		compareTags(t, test.expected[0], thriftSpan.Logs[0].GetFields()[0], testName)
		//if test.expectedTimestamp != 0 {
		//	assert.Equal(t, test.expectedTimestamp, thriftSpan.Annotations[0].Timestamp, testName)
		//}
	}
}

func findTag(span *j.Span, key string) *j.Tag {
	for _, s := range span.Tags {
		if s.Key == key {
			return s
		}
	}
	return nil
}

func compareTags(t *testing.T, expected, actual *j.Tag, testName string) {
	assert.Equal(t, expected.Key, actual.Key, testName)
	assert.Equal(t, expected.VType, actual.VType, testName)
	switch expected.VType {
	case j.TagType_STRING:
		assert.Equal(t, *expected.VStr, *actual.VStr, testName)
	case j.TagType_LONG:
		assert.Equal(t, *expected.VLong, *actual.VLong, testName)
	case j.TagType_DOUBLE:
		assert.Equal(t, *expected.VDouble, *actual.VDouble, testName)
	case j.TagType_BOOL:
		assert.Equal(t, *expected.VBool, *actual.VBool, testName)
	case j.TagType_BINARY:
		assert.Equal(t, *expected.VBinary, *actual.VBinary, testName)
	}

}
