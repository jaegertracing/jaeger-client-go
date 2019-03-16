// Copyright (c) 2017 Uber Technologies, Inc.
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

import "sync"

// SpanAllocator abstraction of managign span allocations
type SpanAllocator interface {
	Get() *Span
	Put(*Span)
	IsPool() bool
}

type spanSyncPool struct {
	spanPool sync.Pool
}

func newSpanSyncPool() SpanAllocator {
	return &spanSyncPool{
		spanPool: sync.Pool{New: func() interface{} {
			return &Span{}
		}},
	}
}

func (pool *spanSyncPool) Get() *Span {
	return pool.spanPool.Get().(*Span)
}

func (pool *spanSyncPool) Put(span *Span) {
	span.reset()
	pool.spanPool.Put(span)
}

func (pool *spanSyncPool) IsPool() bool {
	return true
}

type spanSimpleAllocator struct{}

func (pool spanSimpleAllocator) Get() *Span {
	return &Span{}
}

func (pool spanSimpleAllocator) Put(span *Span) {
	span.reset()
}

func (pool spanSimpleAllocator) IsPool() bool {
	return false
}
