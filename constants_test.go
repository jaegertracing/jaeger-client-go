package jaeger

import (
	"strings"
	"testing"
)

func TestHeaderConstants(t *testing.T) {
	if TracerStateHeaderName != strings.ToLower(TracerStateHeaderName) {
		t.Errorf("TracerStateHeaderName is not lower-case: %+v", TracerStateHeaderName)
	}
	if TraceBaggageHeaderPrefix != strings.ToLower(TraceBaggageHeaderPrefix) {
		t.Errorf("TraceBaggageHeaderPrefix is not lower-case: %+v", TraceBaggageHeaderPrefix)
	}
}
