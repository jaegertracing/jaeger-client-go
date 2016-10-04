// Package transport defines various transports that can be used with
// RemoteReporter to send spans out of process. Transport is responsible
// for serializing the spans into a specific format suitable for sending
// to the tracing backend. Examples may include Thrift over UDP, Thrift
// or JSON over HTTP, Thrift over Kafka, etc.
//
// Implementations are NOT required to be thread-safe; the RemoteReporter
// is expected to only call methods on the Transport from the same go-routine.
package transport
