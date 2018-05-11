// Copyright (c) 2018 The Jaeger Authors.
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

package zipkin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crossdock/crossdock-go/assert"
	zipkinModel "github.com/openzipkin/zipkin-go/model"
	zipkinHttp "github.com/openzipkin/zipkin-go/reporter/http"
	jaeger "github.com/uber/jaeger-client-go"
)

func TestZipkinReporter(t *testing.T) {
	var spansReceived []zipkinModel.SpanModel

	// Inspired by
	// https://github.com/openzipkin/zipkin-go/blob/master/reporter/http/http_test.go
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected 'POST' request, got '%s'", r.Method)
		}

		if spansReceived != nil {
			t.Fatalf("received more than one set of spans")
		}

		err := json.NewDecoder(r.Body).Decode(&spansReceived)
		if err != nil {
			t.Fatalf("unexpected error: %s", err.Error())
		}
	}))
	defer ts.Close()

	rep := zipkinHttp.NewReporter(ts.URL)
	defer rep.Close()
	zipkinReporter := &Reporter{
		zipkinHttp.NewReporter(ts.URL),
	}

	tracer, closer := jaeger.NewTracer("DOOP",
		jaeger.NewConstSampler(true),
		zipkinReporter)

	sp := tracer.StartSpan("s1").(*jaeger.Span)
	sp.SetTag("test", "abc")
	sp.Finish()

	sp2 := tracer.StartSpan("s2").(*jaeger.Span)
	sp2.Finish()

	closer.Close()

	assert.Len(t, spansReceived, 2)
	assert.Equal(t, "abc", spansReceived[0].Tags["test"])
	assert.Equal(t, "s2", spansReceived[1].Name)
}
