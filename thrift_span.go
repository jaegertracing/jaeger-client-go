// Copyright (c) 2016 Uber Technologies, Inc.
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
	"encoding/binary"
	"fmt"
	"time"

	"github.com/opentracing/opentracing-go/ext"

	z "github.com/uber/jaeger-client-go/thrift-gen/zipkincore"
)

const (
	// maxAnnotationLength is the max length of byte array or string allowed in the annotations
	maxAnnotationLength = 256

	// Zipkin UI does not work well with non-string tag values
	allowPackedNumbers = false
)

var (
	logPayloadLabels = makeLogPayloadLabels(100)
)

func makeLogPayloadLabels(size int) []string {
	labels := make([]string, size)
	for i := 0; i < size; i++ {
		labels[i] = fmt.Sprintf("log_payload_%d", i)
	}
	return labels
}

// buildThriftSpan builds thrift span based on internal span.
func buildThriftSpan(span *span) *z.Span {
	parentID := int64(span.context.parentID)
	var ptrParentID *int64
	if parentID != 0 {
		ptrParentID = &parentID
	}
	timestamp := timeToMicrosecondsSinceEpochInt64(span.startTime)
	duration := span.duration.Nanoseconds() / int64(time.Microsecond)
	endpoint := &z.Endpoint{
		ServiceName: span.tracer.serviceName,
		Ipv4:        int32(span.tracer.hostIPv4)}
	thriftSpan := &z.Span{
		TraceID:           int64(span.context.traceID),
		ID:                int64(span.context.spanID),
		ParentID:          ptrParentID,
		Name:              span.operationName,
		Timestamp:         &timestamp,
		Duration:          &duration,
		Debug:             span.context.IsDebug(),
		Annotations:       buildAnnotations(span, endpoint),
		BinaryAnnotations: buildBinaryAnnotations(span, endpoint)}
	return thriftSpan
}

func buildAnnotations(span *span, endpoint *z.Endpoint) []*z.Annotation {
	// automatically adding 2 Zipkin CoreAnnotations
	annotations := make([]*z.Annotation, 0, 2+len(span.logs))
	var startLabel, endLabel string
	if span.spanKind == string(ext.SpanKindRPCClientEnum) {
		startLabel, endLabel = z.CLIENT_SEND, z.CLIENT_RECV
	} else if span.spanKind == string(ext.SpanKindRPCServerEnum) {
		startLabel, endLabel = z.SERVER_RECV, z.SERVER_SEND
	}
	if !span.startTime.IsZero() && startLabel != "" {
		start := &z.Annotation{
			Timestamp: timeToMicrosecondsSinceEpochInt64(span.startTime),
			Value:     startLabel,
			Host:      endpoint}
		annotations = append(annotations, start)
		if span.duration != 0 {
			endTs := span.startTime.Add(span.duration)
			end := &z.Annotation{
				Timestamp: timeToMicrosecondsSinceEpochInt64(endTs),
				Value:     endLabel,
				Host:      endpoint}
			annotations = append(annotations, end)
		}
	}
	for _, log := range span.logs {
		anno := &z.Annotation{
			Timestamp: timeToMicrosecondsSinceEpochInt64(log.Timestamp),
			Value:     truncateString(log.Event),
			Host:      endpoint}
		annotations = append(annotations, anno)
	}
	return annotations
}

func buildBinaryAnnotations(span *span, endpoint *z.Endpoint) []*z.BinaryAnnotation {
	// automatically adding local component or server/client address tag, and client version
	annotations := make([]*z.BinaryAnnotation, 0, 2+len(span.tags))

	if span.firstInProcess {
		annotations = append(annotations, &z.BinaryAnnotation{
			Key:            JaegerClientTag,
			Value:          JaegerGoVersion,
			AnnotationType: z.AnnotationType_STRING})
	}

	if span.peerDefined() && span.isRPC() {
		peer := z.Endpoint{
			Ipv4:        span.peer.Ipv4,
			Port:        span.peer.Port,
			ServiceName: span.peer.ServiceName}
		label := z.CLIENT_ADDR
		if span.isRPCClient() {
			label = z.SERVER_ADDR
		}
		anno := &z.BinaryAnnotation{
			Key:            label,
			Value:          []byte{1},
			AnnotationType: z.AnnotationType_BOOL,
			Host:           &peer}
		annotations = append(annotations, anno)
	}
	if !span.isRPC() {
		componentName := endpoint.ServiceName
		for _, tag := range span.tags {
			if tag.key == string(ext.Component) {
				componentName = stringify(tag.value)
				break
			}
		}
		local := &z.BinaryAnnotation{
			Key:            z.LOCAL_COMPONENT,
			Value:          []byte(componentName),
			AnnotationType: z.AnnotationType_STRING,
			Host:           endpoint}
		annotations = append(annotations, local)
	}
	for _, tag := range span.tags {
		if anno := buildBinaryAnnotation(tag.key, tag.value, nil); anno != nil {
			annotations = append(annotations, anno)
		}
	}
	for i, log := range span.logs {
		if log.Payload != nil {
			label := "log_payload"
			if i < len(logPayloadLabels) {
				label = logPayloadLabels[i]
			}
			if anno := buildBinaryAnnotation(label, log.Payload, nil); anno != nil {
				annotations = append(annotations, anno)
			}
		}
	}
	return annotations
}

func buildBinaryAnnotation(key string, val interface{}, endpoint *z.Endpoint) *z.BinaryAnnotation {
	bann := &z.BinaryAnnotation{Key: key, Host: endpoint}
	if value, ok := val.(string); ok {
		bann.Value = []byte(truncateString(value))
		bann.AnnotationType = z.AnnotationType_STRING
	} else if value, ok := val.([]byte); ok {
		if len(value) > maxAnnotationLength {
			value = value[:maxAnnotationLength]
		}
		bann.Value = value
		bann.AnnotationType = z.AnnotationType_BYTES
	} else if value, ok := val.(int32); ok && allowPackedNumbers {
		bann.Value = int32ToBytes(value)
		bann.AnnotationType = z.AnnotationType_I32
	} else if value, ok := val.(int64); ok && allowPackedNumbers {
		bann.Value = int64ToBytes(value)
		bann.AnnotationType = z.AnnotationType_I64
	} else if value, ok := val.(int); ok && allowPackedNumbers {
		bann.Value = int64ToBytes(int64(value))
		bann.AnnotationType = z.AnnotationType_I64
	} else if value, ok := val.(bool); ok {
		bann.Value = []byte{boolToByte(value)}
		bann.AnnotationType = z.AnnotationType_BOOL
	} else {
		value := stringify(val)
		bann.Value = []byte(truncateString(value))
		bann.AnnotationType = z.AnnotationType_STRING
	}
	return bann
}

func stringify(value interface{}) string {
	if s, ok := value.(string); ok {
		return s
	}
	return fmt.Sprintf("%+v", value)
}

func truncateString(value string) string {
	// we ignore the problem of utf8 runes possibly being sliced in the middle,
	// as it is rather expensive to iterate through each tag just to find rune
	// boundaries.
	if len(value) > maxAnnotationLength {
		return value[:maxAnnotationLength]
	}
	return value
}

func boolToByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// Passing time by value is faster than passing a pointer!
// BenchmarkTimeByValue-8	2000000000	         1.37 ns/op
// BenchmarkTimeByPtr-8  	2000000000	         1.98 ns/op
func timeToMicrosecondsSinceEpochInt64(t time.Time) int64 {
	return t.UnixNano() / 1000
}

// int32ToBytes converts int32 to bytes.
func int32ToBytes(i int32) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(i))
	return buf
}

// int64ToBytes converts int64 to bytes.
func int64ToBytes(i int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(i))
	return buf
}
