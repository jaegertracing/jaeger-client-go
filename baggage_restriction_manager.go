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

const (
	defaultMaxValueLength = 2048
)

// baggageRestrictionManager keeps track of valid baggage keys and their size restrictions.
type baggageRestrictionManager interface {
	// setBaggage sets the baggage key:value on a span and the associated logs.
	setBaggage(span *Span, key, value string)
}

// defaultBaggageRestrictionManager allows any baggage key.
type defaultBaggageRestrictionManager struct {
	setter baggageSetter
}

func newDefaultBaggageRestrictionManager(metrics *Metrics, maxValueLength int) *defaultBaggageRestrictionManager {
	if maxValueLength == 0 {
		maxValueLength = defaultMaxValueLength
	}
	return &defaultBaggageRestrictionManager{
		setter: newDefaultBaggageSetter(maxValueLength, metrics),
	}
}

func (m *defaultBaggageRestrictionManager) setBaggage(span *Span, key, value string) {
	m.setter.setBaggage(span, key, value)
}
