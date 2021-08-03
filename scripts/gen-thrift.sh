#!/bin/bash

set -e

PROJECT_ROOT="$(
	cd "$(dirname "$0")/.."
	pwd
)"

IMPORT_PATH=github.com/uber/jaeger-client-go
THRIFT_GEN_DIR=thrift-gen
THRIFT_GO_ARGS=thrift_import="github.com/apache/thrift/lib/go/thrift"

pushd $PROJECT_ROOT 1>/dev/null

thrift -o ./ --gen go:${THRIFT_GO_ARGS} --out ${THRIFT_GEN_DIR} idl/thrift/agent.thrift
thrift -o ./ --gen go:${THRIFT_GO_ARGS} --out ${THRIFT_GEN_DIR} idl/thrift/sampling.thrift
thrift -o ./ --gen go:${THRIFT_GO_ARGS} --out ${THRIFT_GEN_DIR} idl/thrift/jaeger.thrift
thrift -o ./ --gen go:${THRIFT_GO_ARGS} --out ${THRIFT_GEN_DIR} idl/thrift/zipkincore.thrift
thrift -o ./ --gen go:${THRIFT_GO_ARGS} --out ${THRIFT_GEN_DIR} idl/thrift/baggage.thrift
thrift -o ./ --gen go:${THRIFT_GO_ARGS} --out crossdock/thrift/ idl/thrift/crossdock/tracetest.thrift

sed -i 's|"zipkincore"|"'${IMPORT_PATH}'/thrift-gen/zipkincore"|g' ${THRIFT_GEN_DIR}/agent/*.go
sed -i 's|"jaeger"|"'${IMPORT_PATH}'/thrift-gen/jaeger"|g' ${THRIFT_GEN_DIR}/agent/*.go
sed -i 's|"github.com/apache/thrift/lib/go/thrift"|"github.com/uber/jaeger-client-go/thrift"|g' \
	${THRIFT_GEN_DIR}/*/*.go crossdock/thrift/tracetest/*.go

rm -rf thrift-gen/*/*-remote
rm -rf crossdock/thrift/*/*-remote
rm -rf thrift-gen/jaeger/collector.go

popd 1>/dev/null
