//go:build cgo

package cgo

/*
#cgo CFLAGS: -I${SRCDIR}/..
#include <stddef.h>
#include <stdint.h>
#include <string.h>
#include "ntt_port.h"

// Wrappers around the function-pointer fields of NTT_TestPort. cgo
// cannot call C function pointers directly from Go because the cgo
// translator does not generate trampolines for them; the simplest
// portable workaround is a thin C wrapper per hook that does the
// indirect call on the C side.

static int ntt_call_on_map(NTT_TestPort *p) {
    if (p == NULL || p->on_map == NULL) return 0;
    return p->on_map(p->user);
}
static int ntt_call_on_unmap(NTT_TestPort *p) {
    if (p == NULL || p->on_unmap == NULL) return 0;
    return p->on_unmap(p->user);
}
static int ntt_call_on_start(NTT_TestPort *p) {
    if (p == NULL || p->on_start == NULL) return 0;
    return p->on_start(p->user);
}
static int ntt_call_on_stop(NTT_TestPort *p) {
    if (p == NULL || p->on_stop == NULL) return 0;
    return p->on_stop(p->user);
}
static int ntt_call_send(NTT_TestPort *p, const uint8_t *data, size_t len) {
    if (p == NULL || p->send == NULL) return -1;
    NTT_Buffer buf = { data, len };
    return p->send(p->user, buf);
}
static int ntt_call_call(NTT_TestPort *p, const uint8_t *data, size_t len,
                         uint8_t **reply_data, size_t *reply_len) {
    if (p == NULL || p->call == NULL) return -1;
    NTT_Buffer buf = { data, len };
    NTT_Buffer reply = { NULL, 0 };
    int rc = p->call(p->user, buf, &reply);
    if (reply_data) *reply_data = (uint8_t *)reply.data;
    if (reply_len)  *reply_len  = reply.len;
    return rc;
}

// Helper to fish out the port name without exposing the string
// pointer to Go's strict pointer rules.
static const char *ntt_port_name(NTT_TestPort *p) { return p ? p->name : NULL; }

// Inject trampoline: lives in C because NTT_Runtime.inject is a C
// function pointer the test port calls. The trampoline forwards into
// Go via the //export'd ntt_inject_go below, treating the opaque
// handle the bridge stashed at map time as a pointer to the C-side
// port name (stable for the port's lifetime - kept in static storage
// by ntt_port_register).
extern int ntt_inject_go(char *port, uint8_t *data, size_t len);

static int ntt_inject_trampoline(void *handle, NTT_Buffer payload) {
    char *name = (char *)handle;
    if (name == NULL) return -1;
    return ntt_inject_go(name, (uint8_t *)payload.data, payload.len);
}

// Wires the runtime callbacks into a port's NTT_Runtime sub-struct so
// the port's send hook can later call p->runtime.inject(...) to push
// a received payload back into the TTCN-3 in-queue. Idempotent: safe
// to call from OnMap on every (re-)map.
static void ntt_setup_runtime(NTT_TestPort *p) {
    if (p == NULL) return;
    p->runtime.handle = (void *)p->name;
    p->runtime.inject = ntt_inject_trampoline;
}
*/
import "C"

import (
	"context"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
)

// cPort holds the C-side NTT_TestPort pointer for one registered
// port. Wired up by ntt_port_register; consumed by the Go-side
// driver wrapper.
type cPort struct {
	name string
	ptr  *C.NTT_TestPort
}

var (
	cMu        sync.RWMutex
	cByHandle  = map[uint64]*cPort{}
	cByName    = map[string]*cPort{}
)

// storeCPort stashes the C-side pointer for later use by the bridge
// wrappers. The handle is the same uint64 returned by registry.go's
// register() so the two maps stay in sync.
func storeCPort(handle uint64, name string, ptr *C.NTT_TestPort) {
	cMu.Lock()
	defer cMu.Unlock()
	p := &cPort{name: name, ptr: ptr}
	cByHandle[handle] = p
	cByName[name] = p
}

func dropCPort(name string) {
	cMu.Lock()
	defer cMu.Unlock()
	if p, ok := cByName[name]; ok {
		for h, x := range cByHandle {
			if x == p {
				delete(cByHandle, h)
				break
			}
		}
		delete(cByName, name)
	}
}

func loadCPort(name string) *cPort {
	cMu.RLock()
	defer cMu.RUnlock()
	return cByName[name]
}

//export ntt_port_register
func ntt_port_register(p *C.NTT_TestPort) C.int {
	if p == nil || p.name == nil {
		return -1
	}
	name := C.GoString(p.name)
	handle := register(name)
	if handle == 0 {
		return -1 // duplicate
	}
	storeCPort(handle, name, p)
	return 0
}

//export ntt_port_unregister
func ntt_port_unregister(cname *C.char) {
	if cname == nil {
		return
	}
	name := C.GoString(cname)
	dropCPort(name)
	unregister(name)
}

// ntt_inject_go is the Go side of the C inject trampoline declared in
// the cgo preamble. A C test port that received traffic from its
// underlying transport invokes p->runtime.inject(...), which routes
// here and pushes the payload onto the matching TTCN-3 port queue
// (resolved via runtime.CurrentExec() because the C call site has no
// other handle to the executing testcase). Returns 0 on success, -1
// when no testcase is active or the JSON payload is malformed.
//
//export ntt_inject_go
func ntt_inject_go(name *C.char, data *C.uint8_t, length C.size_t) C.int {
	debug := os.Getenv("NTT_PORT_DEBUG") != ""
	if name == nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[cabi-inject] nil port name\n")
		}
		return -1
	}
	portName := C.GoString(name)
	var payload []byte
	if data != nil && length > 0 {
		payload = C.GoBytes(unsafe.Pointer(data), C.int(length))
	}
	exec := runtime.CurrentExec()
	if exec == nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[cabi-inject] no current exec for port %q\n", portName)
		}
		return -1
	}
	obj, err := decodePayload(payload)
	if err != nil {
		if debug {
			fmt.Fprintf(os.Stderr, "[cabi-inject] decode failed for port %q: %v\n", portName, err)
		}
		return -1
	}
	// The C side only knows the registered port-type name
	// (e.g. "MyClient_PT"). Resolve it back to the TTCN-3
	// instance the testcase actually reads from (e.g. "cli")
	// via the (type -> instance) map runtimeAdapter.Map stashed.
	// Fall through to the raw type name when no instance is
	// recorded, so test setups that register the C port under
	// the TTCN-3 instance name continue to work.
	target := portName
	if inst := runtime.LookupPortTypeInstance(portName); inst != "" {
		target = inst
	}
	if debug {
		fmt.Fprintf(os.Stderr, "[cabi-inject] EnqueueMessageFrom(port=%q, payload-bytes=%d)\n", target, len(payload))
	}
	exec.EnqueueMessageFrom(target, obj, nil)
	return 0
}

// CDriver is the api.TestPort-compatible adapter that fronts a
// C-registered port. It is constructed via NewCDriver and bound to a
// runtime/port.Port through the api.Driver wrapper.
type CDriver struct {
	name string
}

// NewCDriver returns an api.TestPort backed by the C registration
// for name. If no port is registered with that name yet (e.g. the
// C++ constructor that calls ntt_port_register hasn't run), the
// returned driver still works but the hook calls will return
// errPortNotRegistered until the registration arrives.
func NewCDriver(name string) *CDriver { return &CDriver{name: name} }

// Name reports the test port's display name.
func (d *CDriver) Name() string { return d.name }

func (d *CDriver) callHook(fn func(*C.NTT_TestPort) C.int) error {
	p := loadCPort(d.name)
	if p == nil || p.ptr == nil {
		return errPortNotRegistered
	}
	rc := fn(p.ptr)
	if rc != 0 {
		return errHookFailed
	}
	return nil
}

// OnMap calls the C-side on_map hook. The runtime callback table is
// wired up first so the port can already use p->runtime.inject() from
// inside on_map (a few ports lazily kick off a background reader
// thread during map and want to push the first incoming traffic as
// soon as they have it).
func (d *CDriver) OnMap(ctx context.Context) error {
	if os.Getenv("NTT_PORT_DEBUG") != "" {
		fmt.Fprintf(os.Stderr, "[cabi-bridge] OnMap(%q)\n", d.name)
	}
	return d.callHook(func(p *C.NTT_TestPort) C.int {
		C.ntt_setup_runtime(p)
		return C.ntt_call_on_map(p)
	})
}

// OnUnmap calls the C-side on_unmap hook.
func (d *CDriver) OnUnmap(ctx context.Context) error {
	return d.callHook(func(p *C.NTT_TestPort) C.int { return C.ntt_call_on_unmap(p) })
}

// OnStart calls the C-side on_start hook.
func (d *CDriver) OnStart(ctx context.Context) error {
	return d.callHook(func(p *C.NTT_TestPort) C.int { return C.ntt_call_on_start(p) })
}

// OnStop calls the C-side on_stop hook.
func (d *CDriver) OnStop(ctx context.Context) error {
	return d.callHook(func(p *C.NTT_TestPort) C.int { return C.ntt_call_on_stop(p) })
}

// Send marshals the envelope's payload into a NTT_Buffer and hands
// it to the C send hook. Only []byte payloads cross the bridge -
// higher-level encoding lives in Go.
func (d *CDriver) Send(ctx context.Context, env *port.Envelope) error {
	data, ok := payloadBytes(env)
	if !ok {
		return errUnsupportedPayload
	}
	p := loadCPort(d.name)
	if p == nil || p.ptr == nil {
		return errPortNotRegistered
	}
	var cdata *C.uint8_t
	clen := C.size_t(len(data))
	if len(data) > 0 {
		cdata = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	if rc := C.ntt_call_send(p.ptr, cdata, clen); rc != 0 {
		return errHookFailed
	}
	return nil
}

// Call mirrors Send but expects a reply that the C side writes back
// into the supplied NTT_Buffer.
func (d *CDriver) Call(ctx context.Context, env *port.Envelope) error {
	data, ok := payloadBytes(env)
	if !ok {
		return errUnsupportedPayload
	}
	p := loadCPort(d.name)
	if p == nil || p.ptr == nil {
		return errPortNotRegistered
	}
	var cdata *C.uint8_t
	clen := C.size_t(len(data))
	if len(data) > 0 {
		cdata = (*C.uint8_t)(unsafe.Pointer(&data[0]))
	}
	var rdata *C.uint8_t
	var rlen C.size_t
	if rc := C.ntt_call_call(p.ptr, cdata, clen, &rdata, &rlen); rc != 0 {
		return errHookFailed
	}
	if rdata != nil && rlen > 0 && env != nil && env.Reply != nil {
		// Procedure-port reply: copy the C buffer once (the C
		// side is allowed to release it after call() returns)
		// and hand it to the alt scheduler via the envelope's
		// Reply channel. The receiver always reads exactly one
		// value, so a single send is safe.
		b := C.GoBytes(unsafe.Pointer(rdata), C.int(rlen))
		select {
		case env.Reply <- b:
		default:
		}
	}
	return nil
}

// payloadBytes returns the envelope's payload as []byte when it can
// be represented that way. Raw transports stay raw - []byte / string
// payloads pass through unchanged. Anything else (typed records,
// lists, maps, scalars) is JSON-encoded by encodePayload so the C
// port can decode it with any mainstream JSON library. The boolean
// return is true unless the encoder explicitly refused the payload;
// errors during JSON marshalling are surfaced as (nil, false) so the
// caller falls back to errUnsupportedPayload rather than sending
// garbage to the wire.
func payloadBytes(env *port.Envelope) ([]byte, bool) {
	if env == nil {
		return nil, false
	}
	switch v := env.Payload.(type) {
	case nil:
		return nil, true
	case []byte:
		return v, true
	case string:
		return []byte(v), true
	}
	if obj, ok := env.Payload.(runtime.Object); ok {
		b, err := encodePayload(obj)
		if err != nil {
			return nil, false
		}
		return b, true
	}
	return nil, false
}

// Sentinel errors returned by the bridge. Wrapped through
// pseudoError so callers can compare without importing api.
type pseudoError string

func (e pseudoError) Error() string { return string(e) }

var (
	errPortNotRegistered  = pseudoError("ntt cabi: port not registered")
	errHookFailed         = pseudoError("ntt cabi: hook returned non-zero")
	errUnsupportedPayload = pseudoError("ntt cabi: payload type not [] byte")
)
