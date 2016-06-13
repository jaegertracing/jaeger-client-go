namespace java com.uber.jaeger.crossdock.tracetest

enum Transport { HTTP, TCHANNEL }

struct Downstream {
    1: required string serviceName
    2: required string serverRole
    3: required string host
    4: required string port
    5: required Transport transport
    6: optional Downstream downstream
}

struct StartTraceRequest {
    1: required string serverRole  // role of the server (always S1)
    2: required bool sampled
    3: required string baggage
    4: required Downstream downstream
}

struct JoinTraceRequest {
    1: required string serverRole  // role of the server, S2 or S3
    2: optional Downstream downstream
}

struct ObservedSpan {
    1: required string traceId
    2: required bool sampled
    3: required string baggage
}

/**
 * Each server must include the information about the span it observed.
 * It can only be omitted from the response if notImplementedError field is not empty.
 * If the server was instructed to make a downstream call, it must embed the
 * downstream response in its own response.
 */
struct TraceResponse {
    1: optional ObservedSpan span
    2: optional TraceResponse downstream
    3: required string notImplementedError
}

service TracedService {
    TraceResponse startTrace(1: StartTraceRequest request)
    TraceResponse joinTrace(1: JoinTraceRequest request)
}
