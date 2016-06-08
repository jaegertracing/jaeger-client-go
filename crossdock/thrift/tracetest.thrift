namespace java com.uber.jaeger.crossdock.tracetest

enum Transport { HTTP, TCHANNEL }

struct Downstream {
    1: required string serviceName
    2: required string host
    3: required string port
    4: required Transport transport
    5: required string clientType
    6: optional Downstream downstream
}

struct StartTraceRequest {
    1: required bool sampled
    2: required string baggage
    3: required Downstream downstream
}

struct JoinTraceRequest {
    1: optional Downstream downstream
}

struct ObservedSpan {
    1: required string traceId
    2: required bool sampled
    3: required string baggage
}

struct TraceResponse {
    1: required ObservedSpan span
    2: optional TraceResponse downstream
}

service TracedService {
    TraceResponse startTrace(1: StartTraceRequest request)
    TraceResponse joinTrace(1: JoinTraceRequest request)
}
