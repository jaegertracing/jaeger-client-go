include "zipkincore.thrift"
namespace java com.uber.jaeger.agent.thrift

service Agent {
    oneway void emitZipkinBatch(1: list<zipkincore.Span> spans)
}
