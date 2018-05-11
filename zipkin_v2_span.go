// Copyright (c) 2018 Uber Technologies, Inc.
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
	"net"

	"github.com/opentracing/opentracing-go/ext"

	zipkinModel "github.com/openzipkin/zipkin-go/model"
	"github.com/uber/jaeger-client-go/internal/spanlog"
	"github.com/uber/jaeger-client-go/utils"
)

// BuildZipkinV2Span converts a Jaeger span to a Zipkin v2 span
func BuildZipkinV2Span(span *Span) *zipkinModel.SpanModel {
	parentID := zipkinModel.ID(span.context.parentID)
	var ptrParentID *zipkinModel.ID
	if parentID != 0 {
		ptrParentID = &parentID
	}

	localEndpoint := &zipkinModel.Endpoint{
		ServiceName: span.tracer.serviceName,
		IPv4:        utils.UnpackUint32AsIP(span.tracer.hostIPv4),
	}

	kind, remoteEndpoint, tags := processTags(span)

	zSpan := &zipkinModel.SpanModel{
		SpanContext: zipkinModel.SpanContext{
			TraceID: zipkinModel.TraceID{
				High: span.context.traceID.High,
				Low:  span.context.traceID.Low,
			},
			ID:       zipkinModel.ID(span.context.spanID),
			ParentID: ptrParentID,
			Debug:    span.context.IsDebug(),
		},
		Name:           span.operationName,
		Timestamp:      span.startTime,
		Duration:       span.duration,
		Kind:           kind,
		LocalEndpoint:  localEndpoint,
		RemoteEndpoint: remoteEndpoint,
		Annotations:    buildZipkinV2Annotations(span),
		Tags:           tags,
	}
	return zSpan
}

func buildZipkinV2Annotations(span *Span) []zipkinModel.Annotation {
	annotations := make([]zipkinModel.Annotation, 0, len(span.logs))
	for _, log := range span.logs {
		anno := zipkinModel.Annotation{
			Timestamp: log.Timestamp,
		}
		if content, err := spanlog.MaterializeWithJSON(log.Fields); err == nil {
			anno.Value = truncateString(string(content))
		} else {
			anno.Value = err.Error()
		}
		annotations = append(annotations, anno)
	}
	return annotations
}

// Handle special tags that get converted to the kind and remote endpoint
// fields, and throw the rest of the tags into a map that becomes the Zipkin
// Tags field.
func processTags(s *Span) (zipkinModel.Kind, *zipkinModel.Endpoint, map[string]string) {
	kind := zipkinModel.Undetermined
	var endpoint *zipkinModel.Endpoint
	tags := make(map[string]string)

	initEndpoint := func() {
		if endpoint == nil {
			endpoint = &zipkinModel.Endpoint{}
		}
	}

	for _, tag := range s.tags {
		switch tag.key {
		case string(ext.PeerHostIPv4):
			initEndpoint()
			endpoint.IPv4 = convertPeerIPv4(tag.value)
		case string(ext.PeerPort):
			initEndpoint()
			endpoint.Port = convertPeerPort(tag.value)
		case string(ext.PeerService):
			initEndpoint()
			endpoint.ServiceName = convertPeerService(tag.value)
		case string(ext.SpanKind):
			switch tag.value {
			case ext.SpanKindRPCClientEnum:
				kind = zipkinModel.Client
			case ext.SpanKindRPCServerEnum:
				kind = zipkinModel.Server
			case ext.SpanKindProducerEnum:
				kind = zipkinModel.Producer
			case ext.SpanKindConsumerEnum:
				kind = zipkinModel.Consumer
			}
		default:
			tags[string(tag.key)] = stringify(tag.value)
		}
	}
	return kind, endpoint, tags
}

func convertPeerIPv4(value interface{}) net.IP {
	if val, ok := value.(string); ok {
		if ip := net.ParseIP(val); ip != nil {
			return ip.To4()
		}
	} else if val, ok := value.(uint32); ok {
		return utils.UnpackUint32AsIP(val)
	} else if val, ok := value.(int32); ok {
		return utils.UnpackUint32AsIP(uint32(val))
	}
	return nil
}

func convertPeerPort(value interface{}) uint16 {
	if val, ok := value.(string); ok {
		if port, err := utils.ParsePort(val); err == nil {
			return port
		}
	}
	if val, ok := value.(uint16); ok {
		return val
	}
	if val, ok := value.(int); ok {
		return uint16(val)
	}
	return 0
}

func convertPeerService(value interface{}) string {
	if val, ok := value.(string); ok {
		return val
	}
	return ""
}
