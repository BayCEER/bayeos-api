package plugin

import (
	"fmt"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   []Plugin
)

// Register adds a plugin to the global registry. It panics on a duplicate name
// so that misuse fails fast at startup.
func Register(p Plugin) {
	if p == nil || p.Name() == "" {
		panic("plugin: cannot register a nil or unnamed plugin")
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	for _, existing := range registry {
		if existing.Name() == p.Name() {
			panic(fmt.Sprintf("plugin: duplicate plugin name %q", p.Name()))
		}
	}
	registry = append(registry, p)
}

// All returns a copy of the registered plugins in registration order.
func All() []Plugin {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Plugin, len(registry))
	copy(out, registry)
	return out
}

// Reset clears the registry. Intended for tests.
func Reset() {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry = registry[:0]
}