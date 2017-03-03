package rpcmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSimpleNameNormalizer(t *testing.T) {
	n := &SimpleNameNormalizer{
		SafeSets: []SafeCharacterSet{
			&Range{From: 'a', To: 'z'},
			&Char{'-'},
		},
		Replacement: '-',
	}
	assert.Equal(t, "ab-cd", n.Normalize("ab-cd"), "all valid")
	assert.Equal(t, "ab-cd", n.Normalize("ab.cd"), "single mismatch")
	assert.Equal(t, "a--cd", n.Normalize("aB-cd"), "range letter mismatch")
}

func TestNormalizedEndpoints(t *testing.T) {
	n := newNormalizedEndpoints(1, DefaultNameNormalizer)

	assertLen := func(l int) {
		n.mux.RLock()
		defer n.mux.RUnlock()
		assert.Len(t, n.names, l)
	}

	assert.Equal(t, "ab-cd", n.normalize("ab^cd"), "one translation")
	assert.Equal(t, "ab-cd", n.normalize("ab^cd"), "cache hit")
	assertLen(1)
	assert.Equal(t, "", n.normalize("xys"), "cache overflow")
}
