package server

import "errors"

// BaggageKey is the key used to pass baggage item
const BaggageKey = "crossdock-baggage-key"

var (
	errNoSpanObserved        = errors.New("no span found in Context")
	errUnrecognizedProtocol  = errors.New("unrecognized protocol for downstream call")
	errCannotStartInTChannel = errors.New("cannot start new trace in tchannel server")
)
