// Copyright (c) 2015 Uber Technologies, Inc.

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

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	assert.NotNil(t, ip, "assert we have an ip")
}

func TestIPToUint32(t *testing.T) {
	intIP, err := IPToUint32("127.0.0.1")
	assert.NoError(t, err)
	assert.Equal(t, uint32((127<<24)|1), intIP, "expected ip is equal to actual ip")

	intIP, err = IPToUint32("127.xxx.0.1")
	assert.Error(t, err)
	assert.EqualValues(t, 0, intIP)

	intIP, err = IPToUint32("")
	assert.Equal(t, ErrEmptyIP, err)
	assert.EqualValues(t, 0, intIP)
}

func TestIPToInt32Error(t *testing.T) {
	ip := "tcollector-21"
	intIP, err := IPToUint32(ip)
	assert.Equal(t, ErrNotFourOctets, err)
	assert.Equal(t, uint32(0), intIP, "expected ip of 0")
}

// Various benchmarks

// Passing time by value or by pointer

func passTimeByValue(t time.Time) int64 {
	return t.UnixNano() / 1000
}

func passTimeByPtr(t *time.Time) int64 {
	return t.UnixNano() / 1000
}

func BenchmarkTimeByValue(b *testing.B) {
	t := time.Now()
	for i := 0; i < b.N; i++ {
		passTimeByValue(t)
	}
}

func BenchmarkTimeByPtr(b *testing.B) {
	t := time.Now()
	for i := 0; i < b.N; i++ {
		passTimeByPtr(&t)
	}
}

// Checking type via casting vs. via .(type)

func checkTypeViaCast(v interface{}) interface{} {
	if vv, ok := v.(int64); ok {
		return vv
	} else if vv, ok := v.(uint64); ok {
		return vv
	} else if vv, ok := v.(int32); ok {
		return vv
	} else if vv, ok := v.(uint32); ok {
		return vv
	}
	return nil
}

func checkTypeViaDotType(v interface{}) interface{} {
	switch v.(type) {
	case int64:
		return v.(int64)
	case uint64:
		return v.(uint64)
	case int32:
		return v.(int32)
	case uint32:
		return v.(uint32)
	default:
		return nil
	}
}

func BenchmarkCheckTypeViaCast(b *testing.B) {
	x := uint32(123)
	for i := 0; i < b.N; i++ {
		checkTypeViaCast(x)
	}
}

func BenchmarkCheckTypeViaDotType(b *testing.B) {
	x := uint32(123)
	for i := 0; i < b.N; i++ {
		checkTypeViaDotType(x)
	}
}
