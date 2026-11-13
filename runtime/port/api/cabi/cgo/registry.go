//go:build cgo

package cgo

import (
	"sort"
	"sync"
)

// registeredPort is the Go-side mirror of a C NTT_TestPort that has
// been handed to the runtime via ntt_port_register. The runtime
// inspects this set during port instantiation; consumers should
// treat it as read-only.
//
// We intentionally do NOT expose the raw C pointer here: the bridge
// keeps it private so callers can't accidentally call into the C
// vtable from the wrong goroutine. The exported methods (Map, Send,
// ...) marshal the call through the bridge.
type registeredPort struct {
	name string
	// handle is an opaque integer the bridge uses to find the
	// matching C-side NTT_TestPort entry. We pass it back to C as
	// part of every call so the bridge can do a fast indexed
	// lookup without touching the global registry map.
	handle uint64
}

// Name returns the test port's display name as passed to
// ntt_port_register.
func (p *registeredPort) Name() string { return p.name }

var (
	regMu   sync.RWMutex
	regByID = map[uint64]*registeredPort{}
	regByNm = map[string]*registeredPort{}
	nextID  uint64
)

// register stores a new port entry and returns the assigned handle.
// Returns 0 if the name is already taken (the C ABI says
// ntt_port_register reports duplicate names with -1).
func register(name string) uint64 {
	regMu.Lock()
	defer regMu.Unlock()
	if _, dup := regByNm[name]; dup {
		return 0
	}
	nextID++
	id := nextID
	p := &registeredPort{name: name, handle: id}
	regByID[id] = p
	regByNm[name] = p
	return id
}

// unregister removes the port identified by name. It is a no-op when
// the name is unknown so the C caller can call it idempotently
// during shutdown.
func unregister(name string) {
	regMu.Lock()
	defer regMu.Unlock()
	if p, ok := regByNm[name]; ok {
		delete(regByID, p.handle)
		delete(regByNm, name)
	}
}

// Registered returns a snapshot of every currently registered port,
// sorted by name. The slice is owned by the caller; the underlying
// registeredPort pointers stay live for the program's lifetime.
func Registered() []*registeredPort {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]*registeredPort, 0, len(regByNm))
	for _, p := range regByNm {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// LookupByName returns the port with the given name, or nil.
func LookupByName(name string) *registeredPort {
	regMu.RLock()
	defer regMu.RUnlock()
	return regByNm[name]
}
