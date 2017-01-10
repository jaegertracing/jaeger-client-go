package main

import (
	"net/http"

	"github.com/uber/jaeger-client-go/crossdock/endtoend/server"
)

const (
	defaultPort = ":8000"
)

func main() {
	handler := &server.Handler{}

	http.HandleFunc("/init", handler.Init)
	http.HandleFunc("/trace", handler.Trace)
	http.ListenAndServe(defaultPort, nil)
}
