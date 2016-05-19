package jaeger

import (
	"io"

	"github.com/uber/jaeger-client-go/thrift/gen/zipkincore"
)

// Sender abstracts the method of sending spans out of process.
// Implementations are NOT required to be thread-safe; the RemoteReporter
// is expected to only call methods on the Sender from the same go-routine.
type Sender interface {
	// Append converts the span to the wire representation and adds it
	// to sender's internal buffer.  If the buffer exceeds its designated
	// size, the Sender should call Flush() and return the number of spans
	// flushed, and any error.
	Append(span *zipkincore.Span) (int, error)

	// Flush submits the internal buffer to the remote server.
	// It returns the number of spans flushed, and any error.
	Flush() (int, error)

	io.Closer
}
