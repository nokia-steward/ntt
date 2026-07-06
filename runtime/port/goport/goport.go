// Package goport binds a TTCN-3 port to a pure-Go test-port
// implementation, with no cgo and no C ABI. It adapts the Titan-style
// api.TestPort interface onto the interpreter's runtime.PortDriver hook
// (and the optional PortController / PortCaller hooks), so a go-only
// program can drive real I/O for the full set of port operations -
// send / receive, map / unmap, start / stop, and procedure call -
// entirely in Go.
//
// Usage from a go-only main package:
//
//	goport.Register("MyPort_PT", func(inst string) api.TestPort {
//	    return &myPort{inst: inst}
//	})
//	// ... then run a testcase whose ports are of type MyPort_PT.
//
// The factory is invoked once per TTCN-3 port INSTANCE (it receives the
// instance name), so distinct instances of the same port type get
// distinct TestPort objects. Outgoing operations are delivered to the
// TestPort; incoming traffic is pushed back into the running testcase
// with goport.Inject from the port's own I/O goroutine.
//
// goport is the cgo-free sibling of runtime/port/api/cabi/cgo: both
// install a runtime.PortDriverProvider, so a single program should use
// one or the other (the last provider installed wins). For a go-only
// environment, import this package - it pulls in no C dependencies and
// leaves go.mod untouched.
package goport

import (
	"context"
	"sync"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/port/api"
)

var (
	mu        sync.Mutex
	factories = map[string]func(instance string) api.TestPort{}
	instances = map[string]api.TestPort{}
	installed bool
	prev      runtime.PortDriverProvider
	hookOnce  sync.Once
)

// Register binds a Go test port to a TTCN-3 port TYPE name (e.g.
// "MyPort_PT"). The factory is called once per port INSTANCE - it
// receives the TTCN-3 instance name - so each instance gets its own
// TestPort. Call it once per type at startup, before running the
// testcase. The first Register installs the global runtime port-driver
// provider.
//
// A test that registers ports should defer Reset to restore the prior
// provider.
func Register(portTypeName string, factory func(instance string) api.TestPort) {
	mu.Lock()
	defer mu.Unlock()
	factories[portTypeName] = factory
	if !installed {
		prev = runtime.SetPortDriverProvider(provide)
		installed = true
	}
	// Drop the per-instance cache at each testcase teardown so
	// real-scheduler component-qualified instances (keyed by a
	// component ID that restarts per testcase) can't alias a stale
	// TestPort across runs. Registered once per process; clearing an
	// already-empty cache is a harmless no-op for non-goport runs.
	hookOnce.Do(func() {
		runtime.RegisterExecTeardownHook(ResetInstances)
	})
}

// ResetInstances drops the per-instance TestPort cache while keeping
// type registrations. Installed as a runtime exec-teardown hook (see
// Register) so each testcase starts with a clean instance table; also
// callable directly from tests. Safe to call between runs — every
// mapped port has already been unmap'd/OnStop'd by the interpreter's
// teardown drain before the hooks fire.
func ResetInstances() {
	mu.Lock()
	defer mu.Unlock()
	instances = map[string]api.TestPort{}
}

// Reset clears every registration and restores the port-driver provider
// that was installed before the first Register. Intended for tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	factories = map[string]func(instance string) api.TestPort{}
	instances = map[string]api.TestPort{}
	if installed {
		runtime.SetPortDriverProvider(prev)
		installed = false
		prev = nil
	}
}

// provide is the runtime-facing resolver consulted on every port
// operation that touches a port whose driver isn't cached yet. It
// matches on the port type name first (the declarative case) and falls
// back to the instance name, then returns (creating on first use) the
// per-instance TestPort wrapped in an adapter.
func provide(portTypeName, portInstName string) runtime.PortDriver {
	mu.Lock()
	defer mu.Unlock()
	f := factories[portTypeName]
	if f == nil {
		f = factories[portInstName]
	}
	if f == nil {
		return nil
	}
	tp := instances[portInstName]
	if tp == nil {
		tp = f(portInstName)
		instances[portInstName] = tp
	}
	return &adapter{tp: tp, typeName: portTypeName, inst: portInstName}
}

// adapter glues a Go api.TestPort onto the runtime.PortDriver interface
// (plus the optional PortController and PortCaller hooks) the interpreter
// consults. It mirrors the cabi/cgo runtimeAdapter, but calls the Go
// methods directly instead of crossing the C ABI.
type adapter struct {
	tp       api.TestPort
	typeName string // port type the driver was registered under
	inst     string // TTCN-3 port instance name
}

var (
	_ runtime.PortDriver     = (*adapter)(nil)
	_ runtime.PortController = (*adapter)(nil)
	_ runtime.PortCaller     = (*adapter)(nil)
)

// Send forwards an outgoing `p.send(v)` payload to the test port. The
// payload is the evaluated TTCN-3 value as a runtime.Object, wrapped in
// a port.Envelope for parity with the C-ABI path.
func (a *adapter) Send(payload runtime.Object, sender runtime.Object) error {
	return a.tp.Send(context.Background(), &port.Envelope{Payload: payload})
}

// Map runs on `map(self:p, system:p)`: it records the port-type ->
// instance binding (so Inject can route received values by type name)
// and invokes the test port's OnMap so it can open its transport.
func (a *adapter) Map(local, remote string) error {
	inst := a.inst
	if inst == "" {
		inst = local
	}
	runtime.RegisterPortTypeInstance(a.typeName, inst)
	return a.tp.OnMap(context.Background())
}

// Unmap runs on `unmap` / teardown and invokes the test port's OnUnmap.
func (a *adapter) Unmap(local, remote string) error {
	return a.tp.OnUnmap(context.Background())
}

// Start runs on TTCN-3 `p.start` (PortController) and invokes OnStart.
func (a *adapter) Start(localPort string) error {
	return a.tp.OnStart(context.Background())
}

// Stop runs on TTCN-3 `p.stop` / `p.halt` (PortController) and invokes
// OnStop.
func (a *adapter) Stop(localPort string) error {
	return a.tp.OnStop(context.Background())
}

// Call runs on procedure-based `p.call(...)` (PortCaller). It hands the
// call to the test port via an Envelope carrying a one-shot Reply
// channel; a return value the port writes there synchronously becomes
// the reply a later `getreply` reads. A port that replies asynchronously
// instead pushes its reply through the receive path; here we return nil
// (no synchronous reply).
func (a *adapter) Call(payload runtime.Object, sender runtime.Object) (runtime.Object, error) {
	reply := make(chan interface{}, 1)
	env := &port.Envelope{Payload: payload, Reply: reply}
	if err := a.tp.Call(context.Background(), env); err != nil {
		return nil, err
	}
	select {
	case r := <-reply:
		if obj, ok := r.(runtime.Object); ok {
			return obj, nil
		}
		return nil, nil
	default:
		return nil, nil
	}
}

// Inject delivers an incoming MESSAGE from a Go test port to the running
// testcase, as if it had arrived on the named TTCN-3 port - the value a
// `p.receive` reads. name is the port INSTANCE name (preferred; the
// factory hands it to each port) or the registered port-TYPE name
// (resolved to the mapped instance). payload is the TTCN-3 value as a
// runtime.Object.
//
// Call Inject from the test port's own I/O goroutine whenever the SUT
// delivers data. It returns false when no testcase is currently
// executing (so a late delivery after teardown is dropped rather than
// panicking).
func Inject(name string, payload runtime.Object) bool {
	exec := runtime.CurrentExec()
	if exec == nil {
		return false
	}
	exec.EnqueueMessageFrom(resolveTarget(name), payload, nil)
	return true
}

// InjectReply delivers a procedure REPLY from a Go test port - the value
// a `p.getreply` reads (procedure-based comm, ETSI 22.3). ret is the
// return value, bound by `getreply ... -> value v`. params becomes the
// reply's parameter record (matched by a `getreply(S:{...})` template);
// pass nil when the signature has none. Use this instead of the
// synchronous env.Reply channel when the reply arrives asynchronously.
// name follows the same resolution as Inject.
//
// Note: binding `out`/`inout` parameters back via a `getreply ... ->
// param(v := p)` redirect is not modelled for a driver reply - prefer
// the return value (`-> value`) for now.
func InjectReply(name string, params, ret runtime.Object) bool {
	exec := runtime.CurrentExec()
	if exec == nil {
		return false
	}
	exec.EnqueueEnvelope(resolveTarget(name), runtime.PortMessage{
		Kind:     runtime.MsgReply,
		Payload:  params,
		RetValue: ret,
	})
	return true
}

// InjectException delivers a procedure EXCEPTION from a Go test port -
// the value a `p.catch` reads and binds via `-> value v` (the raise/catch
// path, ETSI 22.3). exc is the exception value as a runtime.Object. name
// follows the same resolution as Inject.
func InjectException(name string, exc runtime.Object) bool {
	exec := runtime.CurrentExec()
	if exec == nil {
		return false
	}
	exec.EnqueueEnvelope(resolveTarget(name), runtime.PortMessage{
		Kind:     runtime.MsgException,
		RetValue: exc,
	})
	return true
}

// resolveTarget maps a registered port-type name to its mapped instance
// (recorded at Map time), or passes an instance name through unchanged.
func resolveTarget(name string) string {
	if inst := runtime.LookupPortTypeInstance(name); inst != "" {
		return inst
	}
	return name
}
