package common

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/uber/jaeger-client-go/crossdock/thrift/tracetest"
	"github.com/uber/jaeger-client-go/utils"

	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"golang.org/x/net/context"
)

// PostJSON sends a POST request to `url` with body containing JSON-serialized `req`.
// It injects tracing span into the headers (if found in the context).
// It returns parsed TraceResponse, or error.
func PostJSON(ctx context.Context, url string, req interface{}) (*tracetest.TraceResponse, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	span, err := injectSpan(ctx, httpReq)
	if span != nil {
		defer span.Finish()
	}
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	var result tracetest.TraceResponse
	err = utils.ReadJSON(resp, &result)
	return &result, err
}

func injectSpan(ctx context.Context, req *http.Request) (opentracing.Span, error) {
	span := opentracing.SpanFromContext(ctx)
	if span == nil {
		return nil, nil
	}
	span = opentracing.StartChildSpan(span, "post")
	ext.SpanKind.Set(span, ext.SpanKindRPCClient)
	c := opentracing.HTTPHeaderTextMapCarrier(req.Header)
	err := span.Tracer().Inject(span, opentracing.TextMap, c)
	return span, err
}
