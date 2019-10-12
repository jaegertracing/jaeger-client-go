// Copyright (c) 2019 Uber Technologies, Inc.
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

type SamplingDecision struct {
	sample    bool
	retryable bool
	tags      []Tag
}

type SamplerV2 interface {
	OnCreateSpan(span *Span) SamplingDecision
	OnSetOperationName(span *Span, operationName string) SamplingDecision
	OnSetTag(span *Span, key string, value interface{}) SamplingDecision

	// Close does a clean shutdown of the sampler, stopping any background
	// go-routines it may have started.
	Close()
}

// samplerV1toV2 wraps legacy V1 sampler into an adapter that make it look like V2.
func samplerV1toV2(s Sampler) SamplerV2 {
	if s2, ok := s.(SamplerV2); ok {
		return s2
	}
	type legacySamplerV1toV2Adapter struct {
		legacySamplerV1Base
	}
	return &legacySamplerV1toV2Adapter{
		legacySamplerV1Base: legacySamplerV1Base{
			delegate: s.IsSampled,
		},
	}
}

// legacySamplerV1Base is used as a base for simple samplers that only implement
// the legacy isSampled() function that is not sensitive to its arguments.
type legacySamplerV1Base struct {
	delegate func(id TraceID, operation string) (sampled bool, tags []Tag)
}

func (s *legacySamplerV1Base) OnCreateSpan(span *Span) SamplingDecision {
	isSampled, tags := s.delegate(span.context.traceID, span.operationName)
	return SamplingDecision{sample: isSampled, retryable: false, tags: tags}
}

func (s *legacySamplerV1Base) OnSetOperationName(span *Span, operationName string) SamplingDecision {
	isSampled, tags := s.delegate(span.context.traceID, span.operationName)
	return SamplingDecision{sample: isSampled, retryable: false, tags: tags}
}

func (s *legacySamplerV1Base) OnSetTag(span *Span, key string, value interface{}) SamplingDecision {
	return SamplingDecision{sample: false, retryable: true}
}

func (s *legacySamplerV1Base) Close() {}
