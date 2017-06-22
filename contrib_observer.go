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
	opentracing "github.com/opentracing/opentracing-go"
)

// ContribObserver can be registered with the Tracer to receive notifications
// about new Spans. Modelled after github.com/opentracing-contrib/go-observer.
type ContribObserver interface {
	OnStartSpan(sp opentracing.Span, operationName string, options opentracing.StartSpanOptions) ContribSpanObserver
}

// ContribSpanObserver is created by the Observer and receives notifications
// about other Span events.
type ContribSpanObserver interface {
	OnSetOperationName(operationName string)
	OnSetTag(key string, value interface{})
	OnFinish(options opentracing.FinishOptions)
}

// wrapper observer for the old observers (see observer.go)
type oldObserver struct {
	obs Observer
}

func (o oldObserver) OnStartSpan(sp opentracing.Span, operationName string, options opentracing.StartSpanOptions) ContribSpanObserver {
	return o.obs.OnStartSpan(operationName, options)
}

// contribObserver is a dispatcher to other observers
type contribObserver struct {
	observers []ContribObserver
}

// contribSpanObserver is a dispatcher to other span observers
type contribSpanObserver struct {
	observers []ContribSpanObserver
}

// noopContribSpanObserver is used when there are no observers registered
// on the Tracer or none of them returns span observers from OnStartSpan.
var noopContribSpanObserver = contribSpanObserver{}

func (o *contribObserver) append(contribObserver ContribObserver) {
	o.observers = append(o.observers, contribObserver)
}

func (o contribObserver) OnStartSpan(sp opentracing.Span, operationName string, options opentracing.StartSpanOptions) ContribSpanObserver {
	var spanObservers []ContribSpanObserver
	for _, obs := range o.observers {
		spanObs := obs.OnStartSpan(sp, operationName, options)
		if spanObs != nil {
			if spanObservers == nil {
				spanObservers = make([]ContribSpanObserver, 0, len(o.observers))
			}
			spanObservers = append(spanObservers, spanObs)
		}
	}
	if len(spanObservers) == 0 {
		return noopContribSpanObserver
	}
	return contribSpanObserver{observers: spanObservers}
}

func (o contribSpanObserver) OnSetOperationName(operationName string) {
	for _, obs := range o.observers {
		obs.OnSetOperationName(operationName)
	}
}

func (o contribSpanObserver) OnSetTag(key string, value interface{}) {
	for _, obs := range o.observers {
		obs.OnSetTag(key, value)
	}
}

func (o contribSpanObserver) OnFinish(options opentracing.FinishOptions) {
	for _, obs := range o.observers {
		obs.OnFinish(options)
	}
}
