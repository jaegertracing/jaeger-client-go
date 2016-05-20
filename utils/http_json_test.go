package utils

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testJSONStruct struct {
	Name string
	Age  int
}

func TestGetJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte("{\"name\": \"Bender\", \"age\": 3}"))
	}))
	defer server.Close()

	var s testJSONStruct
	err := GetJSON(server.URL, &s)
	require.NoError(t, err)

	assert.Equal(t, "Bender", s.Name)
	assert.Equal(t, 3, s.Age)
}

func TestGetJSONErrors(t *testing.T) {
	var s testJSONStruct
	err := GetJSON("localhost:0", &s)
	assert.Error(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "some error", http.StatusInternalServerError)
	}))
	defer server.Close()

	err = GetJSON(server.URL, &s)
	assert.Error(t, err)
}
