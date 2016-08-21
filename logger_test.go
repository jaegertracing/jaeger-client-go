package jaeger

import (
	"testing"
)

func TestLogger(t *testing.T) {
	for _, logger := range []Logger{StdLogger, NullLogger} {
		logger.Infof("Hi %s", "there")
		logger.Error("Bad wolf")
	}
}
