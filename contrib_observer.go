// Copyright (c) 2017 Uber Technologies, Inc.
//           (c) 2017 IBM Corp.
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

	otobserver "github.com/opentracing-contrib/go-observer"
)

// wrapper observer for the old observers (see observer.go)
type OldObserver struct {
	Obs Observer
}

func (o OldObserver) OnStartSpan(sp opentracing.Span, operationName string, options opentracing.StartSpanOptions) otobserver.SpanObserver {
	return o.Obs.OnStartSpan(operationName, options)
}

// contribObserver is a dispatcher to other observers
type contribObserver struct {
	observers []otobserver.Observer
}

// contribSpanObserver is a dispatcher to other span observers
type contribSpanObserver struct {
	observers []otobserver.SpanObserver
}

// noopContribSpanObserver is used when there are no observers registered
// on the Tracer or none of them returns span observers from OnStartSpan.
var noopContribSpanObserver = contribSpanObserver{}

func (o *contribObserver) append(contribObserver otobserver.Observer) {
	o.observers = append(o.observers, contribObserver)
}

func (o contribObserver) OnStartSpan(sp opentracing.Span, operationName string, options opentracing.StartSpanOptions) otobserver.SpanObserver {
	var spanObservers []otobserver.SpanObserver
	for _, obs := range o.observers {
		spanObs := obs.OnStartSpan(sp, operationName, options)
		if spanObs != nil {
			if spanObservers == nil {
				spanObservers = make([]otobserver.SpanObserver, 0, len(o.observers))
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
