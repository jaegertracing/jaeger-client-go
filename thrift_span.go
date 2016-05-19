package jaeger

import (
	"fmt"
	"time"

	z "github.com/uber/jaeger-client-go/thrift/gen/zipkincore"

	"github.com/opentracing/opentracing-go/ext"
)

const (
	// maxAnnotationLength is the max length of byte array or string allowed in the annotations
	maxAnnotationLength = 256

	// Zipkin UI does not work well with non-string tag values
	allowPackedNumbers = false
)

// buildThriftSpan builds thrift span based on internal span.
func buildThriftSpan(span *span) *z.Span {
	parentID := int64(span.parentID)
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
		TraceID:           int64(span.traceID),
		ID:                int64(span.spanID),
		ParentID:          ptrParentID,
		Name:              span.operationName,
		Timestamp:         &timestamp,
		Duration:          &duration,
		Annotations:       buildAnnotations(span, endpoint),
		BinaryAnnotations: buildBinaryAnnotations(span, endpoint)}
	return thriftSpan
}

func buildAnnotations(span *span, endpoint *z.Endpoint) []*z.Annotation {
	// automatically adding 2 Zipkin CoreAnnotations
	annotations := make([]*z.Annotation, 0, 2+len(span.logs))
	var startLabel, endLabel string
	if span.spanKind == string(ext.SpanKindRPCClient) {
		startLabel, endLabel = z.CLIENT_SEND, z.CLIENT_RECV
	} else if span.spanKind == string(ext.SpanKindRPCServer) {
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
		label := z.CLIENT_ADDR
		if span.isRPCClient() {
			label = z.SERVER_ADDR
		}
		peer := &z.BinaryAnnotation{
			Key:            label,
			Value:          []byte{1},
			AnnotationType: z.AnnotationType_BOOL,
			Host:           &span.peer}
		annotations = append(annotations, peer)
	}
	if !span.isRPC() {
		// TODO(yurishkuro) deal with local component name
		// https://github.com/opentracing/opentracing.github.io/issues/75
		componentName := endpoint.ServiceName
		local := &z.BinaryAnnotation{
			Key:            z.LOCAL_COMPONENT,
			Value:          []byte(componentName),
			AnnotationType: z.AnnotationType_STRING,
			Host:           endpoint}
		annotations = append(annotations, local)
	}
	for i := range span.tags {
		if anno := buildBinaryAnnotation(&span.tags[i], nil); anno != nil {
			annotations = append(annotations, anno)
		}
	}
	return annotations
}

func buildBinaryAnnotation(tag *tag, endpoint *z.Endpoint) *z.BinaryAnnotation {
	bann := &z.BinaryAnnotation{Key: tag.key, Host: endpoint}
	if value, ok := tag.value.(string); ok {
		bann.Value = []byte(truncateString(value))
		bann.AnnotationType = z.AnnotationType_STRING
	} else if value, ok := tag.value.([]byte); ok {
		if len(value) > maxAnnotationLength {
			value = value[:maxAnnotationLength]
		}
		bann.Value = value
		bann.AnnotationType = z.AnnotationType_BYTES
	} else if value, ok := tag.value.(int32); ok && allowPackedNumbers {
		bann.Value = int32ToBytes(value)
		bann.AnnotationType = z.AnnotationType_I32
	} else if value, ok := tag.value.(int64); ok && allowPackedNumbers {
		bann.Value = int64ToBytes(value)
		bann.AnnotationType = z.AnnotationType_I64
	} else if value, ok := tag.value.(int); ok && allowPackedNumbers {
		bann.Value = int64ToBytes(int64(value))
		bann.AnnotationType = z.AnnotationType_I64
	} else if value, ok := tag.value.(bool); ok {
		bann.Value = []byte{boolToByte(value)}
		bann.AnnotationType = z.AnnotationType_BOOL
	} else {
		value := fmt.Sprintf("%+v", tag.value)
		bann.Value = []byte(truncateString(value))
		bann.AnnotationType = z.AnnotationType_STRING
	}
	return bann
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
