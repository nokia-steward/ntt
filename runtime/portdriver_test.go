package runtime

import (
	"errors"
	"sync"
	"testing"
)

type fakeDriver struct {
	mu     sync.Mutex
	sent   []Object
	maps   [][2]string
	unmaps [][2]string
	sendEr error
}

func (f *fakeDriver) Send(payload Object, _ Object) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, payload)
	return f.sendEr
}

func (f *fakeDriver) Map(local, remote string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maps = append(f.maps, [2]string{local, remote})
	return nil
}

func (f *fakeDriver) Unmap(local, remote string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unmaps = append(f.unmaps, [2]string{local, remote})
	return nil
}

func TestPortDriverProviderHook(t *testing.T) {
	prev := SetPortDriverProvider(nil)
	t.Cleanup(func() { SetPortDriverProvider(prev) })

	if HasPortDriverProvider() {
		t.Fatalf("provider unexpectedly set")
	}

	drv := &fakeDriver{}
	SetPortDriverProvider(func(typeName, instName string) PortDriver {
		if typeName == "MyClient_PT" {
			return drv
		}
		return nil
	})

	exec := NewTestcaseExec("tc")
	exec.SetPortType("cli", "MyClient_PT")

	if got := exec.PortDriver("cli"); got != drv {
		t.Fatalf("PortDriver(cli) = %v, want %v", got, drv)
	}

	if got := exec.PortDriver("missing"); got != nil {
		t.Fatalf("PortDriver(missing) = %v, want nil", got)
	}

	if got := exec.PortDriver("cli"); got != drv {
		t.Fatalf("cached PortDriver(cli) = %v, want %v", got, drv)
	}

	if err := drv.Send(NewCharstring("hello"), nil); err != nil {
		t.Fatalf("Send returned %v", err)
	}
	if got := len(drv.sent); got != 1 {
		t.Fatalf("driver received %d payloads, want 1", got)
	}
}

func TestPortDriverDisabledWithoutProvider(t *testing.T) {
	prev := SetPortDriverProvider(nil)
	t.Cleanup(func() { SetPortDriverProvider(prev) })

	exec := NewTestcaseExec("tc")
	exec.SetPortType("cli", "MyClient_PT")
	if got := exec.PortDriver("cli"); got != nil {
		t.Fatalf("PortDriver returned %v with no provider; want nil", got)
	}
}

func TestPortDriverSendError(t *testing.T) {
	prev := SetPortDriverProvider(nil)
	t.Cleanup(func() { SetPortDriverProvider(prev) })

	sentinel := errors.New("boom")
	drv := &fakeDriver{sendEr: sentinel}
	SetPortDriverProvider(func(string, string) PortDriver { return drv })

	exec := NewTestcaseExec("tc")
	exec.SetPortType("cli", "X")
	d := exec.PortDriver("cli")
	if d == nil {
		t.Fatalf("expected driver")
	}
	if err := d.Send(NewCharstring("x"), nil); !errors.Is(err, sentinel) {
		t.Fatalf("Send err = %v, want %v", err, sentinel)
	}
}

func TestComponentTypePortsRegistry(t *testing.T) {
	// Verify the module-load-time registry threads port-type
	// bindings into the testcase exec so the cabi/cgo bridge sees
	// `cli -> MyClient_PT` (the integration scenario here: drop the dual-name workaround that registers each
	// port under both type and instance names).
	t.Cleanup(func() {
		componentTypePortsMu.Lock()
		delete(componentTypePorts, "MonitorClientCT")
		componentTypePortsMu.Unlock()
	})
	RegisterComponentTypePort("MonitorClientCT", "cli", "MyClient_PT")
	RegisterComponentTypePort("MonitorClientCT", "srv", "MyServer_PT")

	got := ComponentTypePorts("MonitorClientCT")
	if got["cli"] != "MyClient_PT" || got["srv"] != "MyServer_PT" {
		t.Fatalf("ComponentTypePorts = %v", got)
	}

	// Confirm the returned map is a copy: mutating it does not
	// touch the registry.
	got["cli"] = "tampered"
	if again := ComponentTypePorts("MonitorClientCT"); again["cli"] != "MyClient_PT" {
		t.Fatalf("registry mutated through returned map: %v", again)
	}

	if ComponentTypePorts("UnknownCT") != nil {
		t.Fatalf("unknown component returned non-nil map")
	}
}

func TestPortDriverResolvesByComponentTypeRegistry(t *testing.T) {
	// End-to-end: register a component-type binding, simulate
	// `Comp.create` by populating exec.SetPortType from the
	// registry, and confirm the provider receives the *type*
	// name (not the instance fallback).
	prev := SetPortDriverProvider(nil)
	t.Cleanup(func() {
		SetPortDriverProvider(prev)
		componentTypePortsMu.Lock()
		delete(componentTypePorts, "MonitorClientCT")
		componentTypePortsMu.Unlock()
	})

	RegisterComponentTypePort("MonitorClientCT", "cli", "MyClient_PT")

	drv := &fakeDriver{}
	var seenType, seenInst string
	SetPortDriverProvider(func(typeName, instName string) PortDriver {
		seenType, seenInst = typeName, instName
		if typeName == "MyClient_PT" {
			return drv
		}
		return nil
	})

	exec := NewTestcaseExec("tc")
	for inst, ptype := range ComponentTypePorts("MonitorClientCT") {
		exec.SetPortType(inst, ptype)
	}
	if got := exec.PortDriver("cli"); got != drv {
		t.Fatalf("PortDriver(cli) = %v, want fake driver", got)
	}
	if seenType != "MyClient_PT" || seenInst != "cli" {
		t.Fatalf("provider saw (type=%q, inst=%q), want (MyClient_PT, cli)", seenType, seenInst)
	}
}

func TestPortDriverLookupByInstanceFallback(t *testing.T) {
	prev := SetPortDriverProvider(nil)
	t.Cleanup(func() { SetPortDriverProvider(prev) })

	drv := &fakeDriver{}
	SetPortDriverProvider(func(typeName, instName string) PortDriver {
		// Resolver that only knows the instance name.
		if instName == "cli" {
			return drv
		}
		return nil
	})

	exec := NewTestcaseExec("tc")
	if got := exec.PortDriver("cli"); got != drv {
		t.Fatalf("PortDriver(cli) = %v, want %v", got, drv)
	}
}
