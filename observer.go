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

import opentracing "github.com/opentracing/opentracing-go"

// Observer can be registered with the Tracer to receive notifications about
// new Spans. Note that this interface is now Deprecated, see:
// github.com/opentracing-contrib/go-observer.
type Observer interface {
	OnStartSpan(operationName string, options opentracing.StartSpanOptions) SpanObserver
}

// SpanObserver is created by the Observer and receives notifications about
// other Span events. Note that this interface is now Deprecated, see:
// github.com/opentracing-contrib/go-observer.
type SpanObserver interface {
	OnSetOperationName(operationName string)
	OnSetTag(key string, value interface{})
	OnFinish(options opentracing.FinishOptions)
}

// observer is a dispatcher to other observers
type observer struct {
	observers []Observer
}

// noopSpanObserver is used when there are no observers registered on the Tracer
// or none of them returns span observers from OnStartSpan.
var noopSpanObserver = &compositeSpanObserver{}

func (o *observer) append(observer Observer) {
	o.observers = append(o.observers, observer)
}

func (o observer) OnStartSpan(operationName string, options opentracing.StartSpanOptions) SpanObserver {
	var spanObservers []ContribSpanObserver
	for _, obs := range o.observers {
		spanObs := obs.OnStartSpan(operationName, options)
		if spanObs != nil {
			if spanObservers == nil {
				spanObservers = make([]ContribSpanObserver, 0, len(o.observers))
			}
			spanObservers = append(spanObservers, spanObs)
		}
	}
	if len(spanObservers) == 0 {
		return noopSpanObserver
	}
	return &compositeSpanObserver{observers: spanObservers}
}
