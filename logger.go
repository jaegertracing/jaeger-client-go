package jaeger

import "log"

// Logger provides an abstract interface for logging from Reporters.
// Applications can provide their own implementation of this interface to adapt
// reporters logging to whatever logging library they prefer (stdlib log,
// logrus, go-logging, etc).
type Logger interface {
	// Error logs a message at error priority
	Error(msg string)

	// Infof logs a message at info priority
	Infof(msg string, args ...interface{})
}

// StdLogger is implementation of the Logger interface that delegates to default `log` package
var StdLogger = &stdLogger{}

type stdLogger struct{}

func (l *stdLogger) Error(msg string) {
	log.Printf("ERROR: %s", msg)
}

// Infof logs a message at info priority
func (l *stdLogger) Infof(msg string, args ...interface{}) {
	log.Printf(msg, args...)
}

// NullLogger is implementation of the Logger interface that delegates to default `log` package
var NullLogger = &nullLogger{}

type nullLogger struct{}

func (l *nullLogger) Error(msg string)                      {}
func (l *nullLogger) Infof(msg string, args ...interface{}) {}
