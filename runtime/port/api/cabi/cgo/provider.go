//go:build cgo

package cgo

import (
	"context"
	"fmt"
	"os"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
)

// init wires the cgo bridge into the runtime as the global
// PortDriverProvider. Importing this package (with `import _
// "github.com/nokia/ntt/runtime/port/api/cabi/cgo"`) is therefore
// the only step needed from a binary's main package to enable C/C++
// test ports - the interpreter will automatically consult the
// cgo registry whenever a port operation runs against an instance
// whose declared port type has been registered via
// ntt_port_register().
func init() {
	runtime.SetPortDriverProvider(provideDriver)
}

// provideDriver is the runtime-facing resolver. It is consulted on
// every send / map / unmap that touches a port instance whose driver
// hasn't been cached yet. We try the port type name first (the
// declarative case: a C++ test port registered as "MyClient_PT"
// binds to every TTCN-3 port instance whose type is MyClient_PT)
// and fall back to the instance name (the explicit case: a test
// harness that creates one C port per TTCN-3 instance via the
// instance name).
func provideDriver(portTypeName, portInstName string) runtime.PortDriver {
	debug := os.Getenv("NTT_PORT_DEBUG") != ""
	if rp := LookupByName(portTypeName); rp != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[cabi-provider] match type=%q inst=%q -> %s\n", portTypeName, portInstName, rp.Name())
		}
		return &runtimeAdapter{name: portTypeName, instance: portInstName}
	}
	if rp := LookupByName(portInstName); rp != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[cabi-provider] match inst=%q (no type=%q) -> %s\n", portInstName, portTypeName, rp.Name())
		}
		return &runtimeAdapter{name: portInstName, instance: portInstName}
	}
	if debug {
		fmt.Fprintf(os.Stderr, "[cabi-provider] no match type=%q inst=%q\n", portTypeName, portInstName)
	}
	return nil
}

// runtimeAdapter glues the runtime.PortDriver interface to a CDriver.
// We keep them separate so the CDriver stays focused on the C ABI
// while this type encodes the runtime-side semantics
// (payload-to-Envelope, error mapping, context plumbing).
type runtimeAdapter struct {
	name     string
	instance string
}

// Send marshals payload into a port.Envelope and forwards it through
// the registered C-side send hook. Sender is currently best-effort:
// the C ABI does not yet expose an addressable peer, so we keep it
// in the envelope for symmetry with the loopback path. The interface
// is non-streaming - the C hook returns synchronously when the
// transport has accepted the write.
func (a *runtimeAdapter) Send(payload runtime.Object, _ runtime.Object) error {
	drv := NewCDriver(a.name)
	env := &port.Envelope{Payload: payload}
	return drv.Send(context.Background(), env)
}

// Map invokes the C-side on_map hook for the bound port. Local /
// remote names are accepted for forward compatibility (the C API may
// surface them in a future revision) but currently only the local
// name decides which CDriver to invoke. The (type, instance) pair is
// recorded in runtime.RegisterPortTypeInstance so a later
// runtime.inject() callback firing on the C side - which only knows
// the registered port-type name - can resolve back to the right
// TTCN-3 port instance (and therefore the right
// EnqueueMessageFrom queue) on the current testcase exec.
func (a *runtimeAdapter) Map(local string, _ string) error {
	inst := a.instance
	if inst == "" {
		inst = local
	}
	runtime.RegisterPortTypeInstance(a.name, inst)
	drv := NewCDriver(a.name)
	return drv.OnMap(context.Background())
}

// Unmap invokes the C-side on_unmap hook. The (type, instance) entry
// recorded at Map time is left in place: a later remap should
// re-use it, and a fully torn-down exec is GCed wholesale via
// runtime.ClearPortTypeInstances() from the testcase boundary.
func (a *runtimeAdapter) Unmap(_ string, _ string) error {
	drv := NewCDriver(a.name)
	return drv.OnUnmap(context.Background())
}
