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
	zipkinReporter "github.com/openzipkin/zipkin-go/reporter"
	jaeger "github.com/uber/jaeger-client-go"
)

// Reporter adapts a Zipkin Reporter to a Jaeger reporter
//
// Example:
//
//     import zipkinHttp "github.com/openzipkin/zipkin-go/reporter/http"
//     ...
//
//     zipkinReporter := Reporter{
//         zipkinHttp.NewReporter(zipkinUrl)
//     }
//
// Will give you a Jaeger reporter that can be used with the tracer.
type Reporter struct {
	zipkinReporter.Reporter
}

// Report converts the Jaeger span to a Zipkin Span and reports it to the
// Zipkin reporter.
func (r *Reporter) Report(span *jaeger.Span) {
	r.Reporter.Send(*jaeger.BuildZipkinV2Span(span))
}

// Close proxies to the Zipkin reporter's close method
func (r *Reporter) Close() {
	r.Reporter.Close()
}
