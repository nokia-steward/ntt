package runtime

import "sync"

// PortDriver is the abstract port-transport hook the interpreter
// consults whenever it would otherwise enqueue / dequeue on a port's
// loopback queue. A nil driver (the default) keeps the existing
// loopback model intact - sends queue up, receives pop the head, no
// I/O happens. A non-nil driver lets an external transport (today: a
// C/C++ test port via the cabi/cgo bridge) attach to a port:
//
//   - Send is called instead of the loopback enqueue when the
//     testcase issues `p.send(...)`. The driver is responsible for
//     serialising the payload and pushing it onto the transport.
//   - Map / Unmap mirror TTCN-3's port-binding operations and are
//     invoked once per `map(self:p, system:p)` / `unmap(self:p, ...)`
//     statement so the driver can open/close the transport.
//
// Drivers do NOT take part in the receive path here: incoming traffic
// is pushed back into the interpreter's port queue via
// TestcaseExec.EnqueueMessageFrom on whatever goroutine the driver
// owns. That way the alt scheduler keeps a single source of truth for
// what's enqueued on a port, regardless of whether the message
// originated from a loopback send, a peer component, or a C test
// port.
type PortDriver interface {
	Send(payload Object, sender Object) error
	Map(localPort, remotePort string) error
	Unmap(localPort, remotePort string) error
}

// PortController is an OPTIONAL interface a PortDriver may also
// implement to receive TTCN-3 port lifecycle operations (`p.start` /
// `p.stop`, ETSI 22.5). The interpreter type-asserts for it and invokes
// the hook only when the bound driver implements it, so plain
// send/map/unmap drivers are unaffected. localPort is the TTCN-3 port
// instance name.
type PortController interface {
	Start(localPort string) error
	Stop(localPort string) error
}

// PortCaller is an OPTIONAL interface a PortDriver may also implement to
// handle procedure-based `p.call(...)` (ETSI 22.3). When the bound
// driver implements it, the interpreter routes the call to the driver
// instead of the loopback responder model and enqueues the returned
// value as the reply a subsequent `getreply` consumes. A nil reply
// enqueues nothing (the driver will deliver the reply asynchronously via
// the receive path, or there is none).
type PortCaller interface {
	Call(payload Object, sender Object) (reply Object, err error)
}

// PortDriverProvider resolves a driver for a (portTypeName,
// portInstanceName) pair. It returns nil when no driver is bound for
// that combination, in which case the interpreter falls back to the
// loopback model.
//
// The provider is consulted on demand, not eagerly: a TTCN-3
// program that only uses loopback ports (the common case in the
// conformance suite) never reaches the hook because the lookup is
// guarded by `portDriverProvider != nil`.
type PortDriverProvider func(portTypeName, portInstName string) PortDriver

var (
	portDriverProviderMu sync.RWMutex
	portDriverProvider   PortDriverProvider
)

// SetPortDriverProvider installs (or removes, when fn is nil) the
// global port-driver lookup. Called from a build-tag-gated init in
// the cabi/cgo bridge so users without cgo never pay for the hook.
//
// Returns the previous provider for testing convenience (so a test
// can install a mock, run, and restore the original).
func SetPortDriverProvider(fn PortDriverProvider) PortDriverProvider {
	portDriverProviderMu.Lock()
	defer portDriverProviderMu.Unlock()
	old := portDriverProvider
	portDriverProvider = fn
	return old
}

// LookupPortDriver resolves a driver for the given port type and
// instance. Returns nil when no provider is registered or when the
// provider returned nil for this pair.
func LookupPortDriver(portTypeName, portInstName string) PortDriver {
	portDriverProviderMu.RLock()
	fn := portDriverProvider
	portDriverProviderMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(portTypeName, portInstName)
}

// HasPortDriverProvider reports whether any provider is installed.
// Used by the interpreter to skip lookup work entirely when no
// drivers are wired (the conformance-suite hot path).
func HasPortDriverProvider() bool {
	portDriverProviderMu.RLock()
	defer portDriverProviderMu.RUnlock()
	return portDriverProvider != nil
}
