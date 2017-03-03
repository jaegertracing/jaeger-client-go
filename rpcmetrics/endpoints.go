package rpcmetrics

import "sync"

type normalizedEndpoints struct {
	names       map[string]string
	maxSize     int
	defaultName string
	mux         sync.RWMutex
}

func newNormalizedEndpoints(maxSize int) *normalizedEndpoints {
	return &normalizedEndpoints{
		maxSize: maxSize,
		names:   make(map[string]string, maxSize),
	}
}

func (n *normalizedEndpoints) normalize(name string) string {
	n.mux.RLock()
	norm := n.names[name]
	n.mux.RUnlock()
	if norm != "" {
		return norm
	}
	norm = n.safeName(name)
	n.mux.Lock()
	defer n.mux.Unlock()
	if norm, ok := n.names[name]; ok {
		// already added after we released read lock
		return norm
	}
	if len(n.names) >= n.maxSize {
		return ""
	}
	n.names[name] = norm
	return norm
}

// safeName ensures that the name is in the safe charset [a-zA-Z0-9-./_].
// Any characters not in this set are replaced with '-'.
func (n *normalizedEndpoints) safeName(name string) string {
	var retMe []byte
	nameBytes := []byte(name)
	for i, b := range nameBytes {
		if n.safeByte(b) {
			if retMe != nil {
				retMe[i] = b
			}
		} else {
			if retMe == nil {
				retMe = make([]byte, len(nameBytes))
				copy(retMe[0:i], nameBytes[0:i])
			}
			retMe[i] = '-'
		}
	}
	if retMe == nil {
		return name
	}
	return string(retMe)
}

// safeByte checks if b is in the safe charset [a-zA-Z0-9-./_].
func (n *normalizedEndpoints) safeByte(b byte) bool {
	if b >= '0' && b <= '9' {
		return true
	}
	if b >= 'a' && b <= 'z' {
		return true
	}
	if b >= 'A' && b <= 'Z' {
		return true
	}
	if b == '-' || b == '_' || b == '/' || b == '.' {
		return true
	}
	return false
}
