// Package api defines the stable Go-native test-port API. A test port
// is the bridge between the abstract TTCN-3 port model in
// `runtime/port` and the concrete SUT (system under test): it
// translates outgoing TTCN-3 send / call operations into transport
// I/O, and lifts incoming traffic from the transport into TTCN-3
// envelopes the alt scheduler can consume.
//
// The interface intentionally mirrors the structure of Titan's
// TTCN_Port class so existing test ports can be ported with a
// mechanical translation:
//   Titan                         | ntt
//   ------------------------------+--------------------------
//   user_map / user_unmap         | OnMap / OnUnmap
//   user_start / user_stop        | OnStart / OnStop
//   outgoing_send / outgoing_call | Send / Call
//   incoming_message              | Inject (called from the port driver)
//
// A C-ABI shim that exposes these methods to existing Titan-style
// .cc/.hh test ports lives in `runtime/port/api/cabi`; it builds with
// cgo so the Go module stays cgo-free for users that don't need it.
package api

import (
	"context"

	"github.com/nokia/ntt/runtime/port"
)

// TestPort is the contract a custom test port implementation honours.
// All methods are called from the runtime goroutine that owns the
// port; implementations don't need to be goroutine-safe internally,
// but they MUST be non-blocking (or use Inject from a dedicated I/O
// goroutine the implementation spawns itself).
type TestPort interface {
	// Name returns the test port's display name. Used for diagnostics.
	Name() string

	// OnMap is invoked when the TTCN-3 `map(self:p, system:sp)` runs
	// for this port. Implementations open whatever transport they
	// represent here (sockets, hardware handles, ...).
	OnMap(ctx context.Context) error

	// OnUnmap is the inverse of OnMap. Called on `unmap` or on test
	// teardown.
	OnUnmap(ctx context.Context) error

	// OnStart is invoked when TTCN-3 `start <port>` runs. Most ports
	// don't need to differentiate between Map and Start, in which case
	// they can leave this as a no-op.
	OnStart(ctx context.Context) error

	// OnStop is the inverse of OnStart.
	OnStop(ctx context.Context) error

	// Send translates an outgoing TTCN-3 value into transport I/O. The
	// runtime hands over the runtime.port.Envelope so implementations
	// can access the From field for symmetric protocols.
	Send(ctx context.Context, env *port.Envelope) error

	// Call is the procedure-port variant of Send: the runtime expects a
	// reply on env.Reply before the next alt iteration. Message-based
	// ports leave Call unimplemented (returning ErrNotProcedure).
	Call(ctx context.Context, env *port.Envelope) error
}

// Driver wires a TestPort up to a runtime.port.Port. It owns the
// goroutine that pumps Send calls from the runtime to the TestPort and
// makes incoming traffic available on the runtime port's in-queue.
type Driver struct {
	Port     *port.Port
	TestPort TestPort
}

// New constructs a Driver that exposes tp as the local end of p.
func New(p *port.Port, tp TestPort) *Driver { return &Driver{Port: p, TestPort: tp} }

// Inject pushes incoming traffic from the test port into the runtime's
// in-queue. Test port implementations call this from their I/O
// goroutine whenever the SUT delivers data.
func (d *Driver) Inject(payload interface{}) { d.Port.Enqueue(payload) }

// Map invokes the test port's OnMap.
func (d *Driver) Map(ctx context.Context) error { return d.TestPort.OnMap(ctx) }

// Unmap invokes the test port's OnUnmap.
func (d *Driver) Unmap(ctx context.Context) error { return d.TestPort.OnUnmap(ctx) }

// Start invokes the test port's OnStart.
func (d *Driver) Start(ctx context.Context) error { return d.TestPort.OnStart(ctx) }

// Stop invokes the test port's OnStop.
func (d *Driver) Stop(ctx context.Context) error { return d.TestPort.OnStop(ctx) }

// Send routes the envelope through the TestPort.
func (d *Driver) Send(ctx context.Context, env *port.Envelope) error {
	return d.TestPort.Send(ctx, env)
}

// Call routes the procedure-call envelope through the TestPort.
func (d *Driver) Call(ctx context.Context, env *port.Envelope) error {
	return d.TestPort.Call(ctx, env)
}

// Base provides default no-op implementations of every TestPort hook
// so implementations can embed it and only override what they need.
type Base struct {
	PortName string
}

// Name returns Base.PortName.
func (b Base) Name() string { return b.PortName }

// OnMap returns nil.
func (Base) OnMap(context.Context) error { return nil }

// OnUnmap returns nil.
func (Base) OnUnmap(context.Context) error { return nil }

// OnStart returns nil.
func (Base) OnStart(context.Context) error { return nil }

// OnStop returns nil.
func (Base) OnStop(context.Context) error { return nil }

// Send is a placeholder; embed Base and override to actually send.
func (Base) Send(context.Context, *port.Envelope) error { return ErrSendNotImplemented }

// Call is a placeholder; embed Base and override to actually call.
func (Base) Call(context.Context, *port.Envelope) error { return ErrNotProcedure }

// ErrSendNotImplemented is returned by Base.Send so test authors get
// an explicit error if they forget to override Send.
var ErrSendNotImplemented = pseudoError("api.TestPort.Send: not implemented")

// ErrNotProcedure indicates a Call was invoked on a message-only port.
var ErrNotProcedure = pseudoError("api.TestPort.Call: port is message-based")

type pseudoError string

func (e pseudoError) Error() string { return string(e) }
