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

package behavior

import (
	"fmt"
	"runtime"
)

// Sink records the result of calling different behaviors.
type Sink interface {
	Put(e Entry)
	HasErrors() bool
}

// Skipf records a skipped test.
//
// This may be called multiple times if multiple tests inside a behavior were
// skipped.
func Skipf(s Sink, format string, args ...interface{}) {
	s.Put(Entry{
		Status: Skipped,
		Output: fmt.Sprintf(format, args...),
	})
}

// Errorf records a failed test.
//
// This may be called multiple times if multiple tests inside a behavior
// failed.
func Errorf(s Sink, format string, args ...interface{}) {
	s.Put(Entry{
		Status: Failed,
		Output: fmt.Sprintf(format, args...),
	})
}

// Fatalf records a failed test and stops executing the current behavior.
//
// This may be used to stop executing in case of irrecoverable errors.
func Fatalf(s Sink, format string, args ...interface{}) {
	Errorf(s, format, args...)

	// Exit this goroutine and call any deferred functions
	runtime.Goexit()
}

// Successf records a successful test.
//
// This may be called multiple times for multiple successful tests inside a
// behavior.
func Successf(s Sink, format string, args ...interface{}) {
	s.Put(Entry{
		Status: Passed,
		Output: fmt.Sprintf(format, args...),
	})
}

//////////////////////////////////////////////////////////////////////////////

// entrySink is a sink that keeps track of entries in-order
type entrySink struct{ entries []Entry }

// Put an entry into the EntrySink.
func (e *entrySink) Put(v Entry) {
	e.entries = append(e.entries, v)
}

func (e *entrySink) HasErrors() bool {
	for _, entry := range e.entries {
		if entry.Status != Passed {
			return true
		}
	}
	return false
}
