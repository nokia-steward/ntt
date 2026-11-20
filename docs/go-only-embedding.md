# Embedding ntt in a go-only environment

This guide is for using `ntt` as a **pure-Go TTCN-3 test-execution
engine** — embedding the interpreter in your own Go program and binding
TTCN-3 ports to **Go** test-port implementations, with **no cgo and no
C toolchain**.

`ntt` ships two ways to bind a port to real I/O:

| Path | Package | Build | Use when |
|------|---------|-------|----------|
| **Pure Go** | `runtime/port/goport` | cgo-free | You write your ports in Go (this guide) |
| C-ABI / cgo | `runtime/port/api/cabi/cgo` | needs cgo + a C toolchain | You must reuse existing Titan-style `.cc`/`.hh` C/C++ ports |

Both install the same underlying hook (`runtime.SetPortDriverProvider`),
so a single binary uses **one or the other**. For a go-only environment,
import `runtime/port/goport`; it pulls in no C dependencies and leaves
`go.mod`/`go.sum` untouched. Build with `CGO_ENABLED=0` to be sure.

---

## 1. Run a testcase (no ports)

```go
package main

import (
	"fmt"
	"os"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const src = `module Demo {
    type component C {}
    testcase tc() runs on C { setverdict(pass); }
}`

func main() {
	tree := ttcn3.Parse(src) // or read a .ttcn3 file's contents
	if tree == nil || tree.Err != nil {
		fmt.Fprintln(os.Stderr, "parse error:", tree.Err)
		os.Exit(1)
	}
	// RunTestcase(trees, "Module.testcase") -> (verdict, reason, err)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "Demo.tc")
	if err != nil {
		fmt.Fprintln(os.Stderr, "run error:", err)
		os.Exit(1)
	}
	fmt.Printf("verdict=%s reason=%q\n", v, reason) // -> verdict=pass
	if v != runtime.PassVerdict {
		os.Exit(1)
	}
}
```

Key types: `ttcn3.Parse(string) *ttcn3.Tree`,
`interpreter.RunTestcase([]*ttcn3.Tree, name) (runtime.Verdict, string, error)`,
and verdict constants `runtime.PassVerdict` / `FailVerdict` /
`InconcVerdict` / `NoneVerdict` / `ErrorVerdict`.

---

## 2. Bind a TTCN-3 port to a Go test port

A test port translates outgoing port operations into transport I/O and
lifts incoming traffic back into the testcase. Implement the Titan-style
`api.TestPort` interface (embed `api.Base` for default no-op lifecycle
methods, override what you need), register it for a TTCN-3 **port type**,
and call `goport.Inject` from your I/O goroutine for received data. All
the port operations are wired: `send`/`receive`, `map`/`unmap`,
`start`/`stop`, and procedure `call`. The factory is called once per port
**instance** (it receives the instance name), so distinct instances of
the same type get distinct port objects.

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
	"github.com/nokia/ntt/runtime/port/api"
	"github.com/nokia/ntt/runtime/port/goport"
	"github.com/nokia/ntt/ttcn3"
)

// myPort is a pure-Go test port. Here it just echoes; replace the body
// with real socket / channel / hardware I/O.
type myPort struct {
	api.Base
	inst string // this port's TTCN-3 instance name (from the factory)
}

// OnMap runs on TTCN-3 `map(self:p, system:p)`. Open your transport and
// (if it pushes data asynchronously) spawn the I/O goroutine here.
func (p *myPort) OnMap(ctx context.Context) error { return nil }

// OnUnmap runs on `unmap` / teardown. Close the transport.
func (p *myPort) OnUnmap(ctx context.Context) error { return nil }

// OnStart / OnStop run on TTCN-3 `p.start` / `p.stop` (optional; embed
// api.Base to inherit no-op defaults).
func (p *myPort) OnStart(ctx context.Context) error { return nil }
func (p *myPort) OnStop(ctx context.Context) error  { return nil }

// Send runs on `p.send(v)`. env.Payload is the TTCN-3 value as a
// runtime.Object. Serialise and write it to the transport.
func (p *myPort) Send(ctx context.Context, env *port.Envelope) error {
	v, _ := env.Payload.(runtime.Object)
	// ... write v to your transport ...
	goport.Inject(p.inst, v) // demo: echo it straight back
	return nil
}

// Call runs on procedure-based `p.call(S:{...}){...}`. Answer synchronously
// by writing the return value to env.Reply, or out of band with
// goport.InjectReply(p.inst, params, ret) / goport.InjectException(p.inst, e).
func (p *myPort) Call(ctx context.Context, env *port.Envelope) error {
	if env.Reply != nil {
		env.Reply <- runtime.NewInt(0) // compute the real reply here
	}
	return nil
}

const src = `module Demo {
    type port P message { inout integer }
    type component C { port P p }
    testcase tc() runs on C system C {
        map(self:p, system:p);   // triggers OnMap; binds the Go port
        p.send(7);               // -> myPort.Send
        alt {
            [] p.receive(7) { setverdict(pass); }  // <- goport.Inject
            [] p.receive    { setverdict(fail, "unexpected"); }
        }
    }
}`

func main() {
	// Register once at startup, before running the testcase. The factory
	// is called once per port instance (lazily), and receives the TTCN-3
	// instance name.
	goport.Register("P", func(inst string) api.TestPort { return &myPort{inst: inst} })

	tree := ttcn3.Parse(src)
	if tree == nil || tree.Err != nil {
		fmt.Fprintln(os.Stderr, "parse error:", tree.Err)
		os.Exit(1)
	}
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "Demo.tc")
	if err != nil {
		fmt.Fprintln(os.Stderr, "run error:", err)
		os.Exit(1)
	}
	fmt.Printf("verdict=%s reason=%q\n", v, reason)
	if v != runtime.PassVerdict {
		os.Exit(1)
	}
}
```

### Asynchronous receive

For a real transport that delivers data on its own schedule, spawn a
goroutine in `OnMap` and call `goport.Inject` whenever data arrives. The
testcase's blocking `receive` / `alt` parks and wakes on the injected
value:

```go
func (p *myPort) OnMap(ctx context.Context) error {
	go func() {
		for msg := range p.transport.Incoming() { // your transport
			goport.Inject(p.inst, decode(msg)) // decode -> runtime.Object
		}
	}()
	return nil
}
```

---

## 3. The value model (`runtime.Object`)

Payloads crossing the boundary are `runtime.Object` values — the
interpreter's internal value model. Construct them with the `runtime.New*`
constructors and read them back by type-asserting to the concrete type.

```go
runtime.NewInt(42)          // integer
// charstring, boolean, record, list, ... : see runtime/object.go
```

To inspect a received/sent value generically, `obj.Inspect()` returns its
TTCN-3 text form; type-switch on the concrete types (`runtime.Int`,
`runtime.List`, `runtime.Record`, ...) for structured access. The full
set lives in [`runtime/object.go`](../runtime/object.go).

---

## 4. API reference (`runtime/port/goport`)

- `Register(portTypeName string, factory func(instance string) api.TestPort)`
  — bind a Go test port to a TTCN-3 port type. The factory is called once
  per port **instance** (lazily) and receives the instance name. The first
  Register installs the global provider.
- `Inject(name string, payload runtime.Object) bool` — deliver an
  incoming **message** to the running testcase on the named port (what a
  `p.receive` reads). name is the port **instance** name the factory
  handed you (or the registered type name, resolved to its mapped
  instance). Call from your I/O goroutine. Returns `false` if no testcase
  is running.
- `InjectReply(name string, params, ret runtime.Object) bool` — deliver a
  procedure **reply** (what a `p.getreply` reads); `ret` is the return
  value (`-> value`), `params` the parameter record (nil if none). Use
  this when the reply arrives out of band rather than on `env.Reply`.
- `InjectException(name string, exc runtime.Object) bool` — deliver a
  procedure **exception** (what a `p.catch -> value` reads).
- `Reset()` — clear all registrations and restore the previous provider
  (use `defer goport.Reset()` in tests).

The runtime drives an `api.TestPort` (in `runtime/port/api`: `Name`,
`OnMap`, `OnUnmap`, `OnStart`, `OnStop`, `Send`, `Call`) through these
mappings — embed `api.Base` for no-op defaults and override what you need:

| TTCN-3 | TestPort method | runtime hook |
|--------|-----------------|--------------|
| `map(self:p, system:p)` | `OnMap` | `PortDriver.Map` |
| `unmap(...)` | `OnUnmap` | `PortDriver.Unmap` |
| `p.start` | `OnStart` | `PortController.Start` |
| `p.stop` / `p.halt` | `OnStop` | `PortController.Stop` |
| `p.send(v)` | `Send` (env.Payload) | `PortDriver.Send` |
| `p.call(S:{…}){…}` | `Call` (reply via env.Reply) | `PortCaller.Call` |
| incoming message | `goport.Inject(...)` | `EnqueueMessageFrom` |
| async reply / exception | `goport.InjectReply` / `InjectException` | `EnqueueEnvelope` |

---

## 5. Build

```sh
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go test ./...
```

The pure-Go path links no C and adds no module dependencies; `go.mod`/
`go.sum` are unchanged from upstream `ntt`.

---

## 6. Caveats and current limits

- **One provider per binary.** Don't import both `goport` and the
  cabi/cgo bridge in the same binary — both install
  `runtime.SetPortDriverProvider` and the last one wins. Pick one.
- **Routing of received values.** `Inject(name, …)` routes to the port
  **instance** name. The factory hands each port its instance name — use
  that. The registered **type** name also works once the port has been
  `map`ped (the type→instance binding is recorded at map time).
- **Procedure `call` forms.** Use the blocking `p.call(S:{…}){ [] getreply … }`
  form or `p.call(S:{…}, nowait)` + `getreply` (a bare `p.call(S:{…})` with
  no `nowait` and no response block is not routed to the driver). Reply
  either synchronously on `env.Reply` (return value, from inside `Call`) or
  out of band with `InjectReply` / `InjectException`. **Binding `out`/`inout`
  parameters** back via a `getreply … -> param(v := p)` redirect is not
  modelled for a driver reply — prefer the return value.
- **`p.clear` / `p.halt` nuances** aren't forwarded distinctly (`halt`
  maps to `OnStop`; there is no `OnClear`).
- The higher-level `api.Driver` type in `runtime/port/api` is a separate
  helper (its own `runtime/port.Port` queue) and is **not** the
  integration point used here — bind through `goport` as shown above.

---

## See also

- [`runtime/port/goport/goport.go`](../runtime/port/goport/goport.go) — the binding (well-commented).
- [`runtime/port/goport/goport_test.go`](../runtime/port/goport/goport_test.go) — runnable examples: echo, full lifecycle, async receive, procedure call, async reply, exception.
- [`runtime/port/api/api.go`](../runtime/port/api/api.go) — the `TestPort` / `Base` contract.
- [`runtime/portdriver.go`](../runtime/portdriver.go) — the low-level `PortDriver` hook both paths build on.
