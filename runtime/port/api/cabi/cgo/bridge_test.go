//go:build cgo && (linux || darwin || freebsd)

package cgo

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/port"
)

func bigInt(i int64) *big.Int { return big.NewInt(i) }

func TestRegisterAndSend(t *testing.T) {
	defer FixtureUnregister()
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("ntt_port_register: rc=%d", rc)
	}
	if rc := FixtureRegister(); rc == 0 {
		t.Fatalf("duplicate registration accepted; want -1, got 0")
	}

	d := NewCDriver("fixture")
	if err := d.OnMap(context.Background()); err != nil {
		t.Fatalf("OnMap: %v", err)
	}
	if got := FixtureMapCalls(); got != 1 {
		t.Fatalf("on_map calls: want 1, got %d", got)
	}

	env := &port.Envelope{Payload: []byte("hello")}
	if err := d.Send(context.Background(), env); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := FixtureSendCalls(); got != 1 {
		t.Fatalf("send calls: want 1, got %d", got)
	}
	if got := string(FixturePayload()); got != "hello" {
		t.Fatalf("send payload: want %q, got %q", "hello", got)
	}
}

func TestRegistrySnapshot(t *testing.T) {
	defer FixtureUnregister()
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("register: rc=%d", rc)
	}
	got := Registered()
	if len(got) == 0 {
		t.Fatalf("Registered() returned empty after registration")
	}
	found := false
	for _, p := range got {
		if p.Name() == "fixture" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fixture port missing from Registered() snapshot")
	}
}

func TestUnregisterIsIdempotent(t *testing.T) {
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("register: rc=%d", rc)
	}
	FixtureUnregister()
	FixtureUnregister() // must not panic
	if LookupByName("fixture") != nil {
		t.Fatalf("LookupByName still returns the port after unregister")
	}
}

func TestRuntimeProviderRoundTrip(t *testing.T) {
	defer FixtureUnregister()
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("register: rc=%d", rc)
	}

	// Provider hook must resolve a driver for the registered type
	// name and route a Send through the C-side fixture so the call
	// count goes up. This is the integration point the
	// interpreter relies on: PortDriver(instance) -> Send.
	drv := runtime.LookupPortDriver("fixture", "anyInstance")
	if drv == nil {
		t.Fatalf("provider returned nil for known type %q", "fixture")
	}
	before := FixtureSendCalls()
	if err := drv.Send(runtime.NewCharstring("payload"), nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := FixtureSendCalls(); got != before+1 {
		t.Fatalf("FixtureSendCalls = %d, want %d", got, before+1)
	}
	if err := drv.Map("anyInstance", "anyInstance"); err != nil {
		t.Fatalf("Map: %v", err)
	}
}

func TestRuntimeProviderRejectsUnknown(t *testing.T) {
	if drv := runtime.LookupPortDriver("nope_unknown", "nope_unknown"); drv != nil {
		t.Fatalf("provider returned %v for unknown name; want nil", drv)
	}
}

func TestEncodePayloadJSON(t *testing.T) {
	rec := runtime.NewRecord()
	rec.Set("path", runtime.NewCharstring("/probe/liveness"))
	rec.Set("status", runtime.Int{Int: bigInt(200)})

	env := &port.Envelope{Payload: rec}
	got, ok := payloadBytes(env)
	if !ok {
		t.Fatalf("payloadBytes returned !ok for record payload")
	}
	want := `{"path":"/probe/liveness","status":200}`
	if string(got) != want {
		t.Fatalf("encoded payload = %s, want %s", got, want)
	}
}

func TestInjectRoundTrip(t *testing.T) {
	// End-to-end of the inject trampoline: CDriver.OnMap wires
	// the C-side NTT_Runtime substruct so the fixture port can
	// call p->runtime.inject(...) and have it land on the
	// active TestcaseExec's port queue. This is the contract
	// this integration scenario covers: a C port
	// pushing a response back into the TTCN-3 in-queue without
	// any Go-side glue beyond the bridge.
	defer FixtureUnregister()
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("ntt_port_register: rc=%d", rc)
	}

	d := NewCDriver("fixture")
	if err := d.OnMap(context.Background()); err != nil {
		t.Fatalf("OnMap: %v", err)
	}

	// Without an instance-name mapping, inject(<port-type>)
	// routes straight to a queue keyed by the port type. The
	// next test exercises the type -> instance routing path.
	runtime.ClearPortTypeInstances()
	exec := runtime.NewTestcaseExec("inject-tc")
	prev := runtime.CurrentExec()
	runtime.SetCurrentExec(exec)
	t.Cleanup(func() { runtime.SetCurrentExec(prev) })

	payload := []byte(`{"status":200,"body":"ok"}`)
	if rc := FixtureInjectViaRuntime(payload); rc != 0 {
		t.Fatalf("fx_inject_via_runtime: rc=%d (want 0; -2 means OnMap did not wire NTT_Runtime)", rc)
	}

	msg, ok := exec.DequeueMessageFull("fixture")
	if !ok {
		t.Fatalf("port queue empty after inject; expected EnqueueMessageFrom to have landed a message")
	}
	rec, ok := msg.Payload.(*runtime.Record)
	if !ok {
		t.Fatalf("payload type = %T, want *runtime.Record", msg.Payload)
	}
	status, _ := rec.Get("status")
	if status == nil || status.Inspect() != "200" {
		t.Fatalf("status field = %v, want 200", status)
	}
	body, _ := rec.Get("body")
	if body == nil || body.Inspect() != `"ok"` {
		t.Fatalf("body field = %v, want \"ok\"", body)
	}
}

func TestInjectRoutesViaTypeInstanceMap(t *testing.T) {
	// runtimeAdapter.Map() registers (type -> instance) so an
	// inject(<type>) from the C side ends up on the right
	// instance queue. This is the path the external test-port
	// suite hits: port MyClient_PT cli => map records
	// "MyClient_PT" -> "cli"; the C port injects under
	// "MyClient_PT" and we want the message to land in the
	// "cli" queue so cli.receive(...) sees it.
	defer FixtureUnregister()
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("register: rc=%d", rc)
	}
	d := NewCDriver("fixture")
	if err := d.OnMap(context.Background()); err != nil {
		t.Fatalf("OnMap: %v", err)
	}
	runtime.ClearPortTypeInstances()
	runtime.RegisterPortTypeInstance("fixture", "cli")
	t.Cleanup(runtime.ClearPortTypeInstances)

	exec := runtime.NewTestcaseExec("route-tc")
	prev := runtime.CurrentExec()
	runtime.SetCurrentExec(exec)
	t.Cleanup(func() { runtime.SetCurrentExec(prev) })

	if rc := FixtureInjectViaRuntime([]byte(`{"ok":true}`)); rc != 0 {
		t.Fatalf("inject: rc=%d", rc)
	}
	if _, ok := exec.DequeueMessageFull("fixture"); ok {
		t.Fatalf("payload landed in the type-named queue %q; expected the instance queue", "fixture")
	}
	if _, ok := exec.DequeueMessageFull("cli"); !ok {
		t.Fatalf("payload missing from the instance queue %q", "cli")
	}
}

func TestInjectWithoutCurrentExec(t *testing.T) {
	// Defensive path: if no testcase is active when a C port
	// injects, the dispatcher must return -1 instead of panicking.
	defer FixtureUnregister()
	if rc := FixtureRegister(); rc != 0 {
		t.Fatalf("register: rc=%d", rc)
	}
	d := NewCDriver("fixture")
	if err := d.OnMap(context.Background()); err != nil {
		t.Fatalf("OnMap: %v", err)
	}

	prev := runtime.CurrentExec()
	runtime.SetCurrentExec(nil)
	t.Cleanup(func() { runtime.SetCurrentExec(prev) })

	if rc := FixtureInjectViaRuntime([]byte(`{}`)); rc != -1 {
		t.Fatalf("inject without exec: rc=%d, want -1", rc)
	}
}

func TestFdEventLoop(t *testing.T) {
	rfd, wfd, rc := FixturePipe()
	if rc != 0 {
		t.Fatalf("pipe(2): rc=%d", rc)
	}
	defer FixtureClose(rfd)
	defer FixtureClose(wfd)
	defer FixtureRemoveFd(rfd)

	if rc := FixtureRegisterFdRead(rfd); rc != 0 {
		t.Fatalf("add_fd_read: rc=%d", rc)
	}
	if n := FixtureWriteByte(wfd); n != 1 {
		t.Fatalf("write: want 1, got %d", n)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if FixtureFdCallbackCalls() > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := FixtureFdCallbackCalls(); got == 0 {
		t.Fatalf("fd callback never fired after write")
	}
	if got := FixtureFdCallbackFd(); got != rfd {
		t.Fatalf("callback fd: want %d, got %d", rfd, got)
	}
}
