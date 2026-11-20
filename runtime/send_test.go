package runtime_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime"
)

// sendConfig is an MTC (1) whose port p is connected to port q of two
// PTCs (2, 3), whose port array a is connected as a whole to the array a
// of PTC 2, and whose port m is mapped to the system.
func sendConfig() (*runtime.TestcaseExec, *runtime.ComponentRef) {
	exec := runtime.NewTestcaseExec("tc")
	exec.SetMTCID(1)
	mtc := newComponent(1, false)
	exec.PushComponent(mtc)
	exec.ConnectPorts(runtime.PortEndpoint{Comp: 1, Port: "p"}, runtime.PortEndpoint{Comp: 2, Port: "q"})
	exec.ConnectPorts(runtime.PortEndpoint{Comp: 1, Port: "p"}, runtime.PortEndpoint{Comp: 3, Port: "q"})
	exec.ConnectPorts(runtime.PortEndpoint{Comp: 1, Port: "a"}, runtime.PortEndpoint{Comp: 2, Port: "a"})
	exec.MapPorts(runtime.PortEndpoint{Comp: 1, Port: "m"}, runtime.PortEndpoint{Comp: runtime.SystemEndpointID, Port: "m"})
	return exec, mtc
}

// A sending operation goes to the ports its port is connected to, only to
// those of the components a to clause addresses when it addresses any of
// them, and else to the port's own queue (ETSI ES 201 873-1 22.2.1).
func TestRoute(t *testing.T) {
	exec, _ := sendConfig()
	q2, q3 := exec.PortKeyFor(2, "q"), exec.PortKeyFor(3, "q")
	for _, c := range []struct {
		name, port string
		ids        map[int64]bool
		want       []string
		addressed  bool
	}{
		{"every peer", "p", nil, []string{q2, q3}, false},
		{"the addressed peer", "p", map[int64]bool{3: true}, []string{q3}, true},
		{"no peer addressed", "p", map[int64]bool{9: true}, []string{q2, q3}, false},
		{"an element of an array connected whole", "a[1]", nil, []string{exec.PortKeyFor(2, "a[1]")}, false},
		{"a port connected to none", "r", nil, []string{exec.PortKey("r")}, false},
	} {
		keys, addressed := exec.Route(c.port, c.ids)
		if !sameKeys(keys, c.want) || addressed != c.addressed {
			t.Errorf("%s: %v, %v; want %v, %v", c.name, keys, addressed, c.want, c.addressed)
		}
	}
}

// A message sent goes on the queues it is routed to, from the running
// component, but on a mapped port with no driver, where it comes back from
// the system; a port connected to several is sent on with a to clause.
func TestSendMessage(t *testing.T) {
	exec, mtc := sendConfig()
	ptc3 := newComponent(3, false)
	q2, q3 := exec.PortKeyFor(2, "q"), exec.PortKeyFor(3, "q")

	if err := exec.SendMessage("p", runtime.NewInt("1"), nil, false); err == nil || !strings.Contains(err.Inspect(), "22.2.1 d") {
		t.Errorf("a send to several without a to clause: %v, want the 22.2.1 d error", err)
	}
	if err := exec.SendMessage("p", runtime.NewInt("2"), ptc3, true); err != nil {
		t.Fatal(err)
	}
	if exec.QueueLen(q2) != 0 || exec.QueueLen(q3) != 1 {
		t.Errorf("a send to PTC 3: queues %d and %d, want 0 and 1", exec.QueueLen(q2), exec.QueueLen(q3))
	}
	if m, _ := exec.DequeueMessageFull(q3); m.Sender != mtc {
		t.Errorf("a send to PTC 3 is from %v, want the MTC", m.Sender)
	}
	for port, want := range map[string]runtime.Object{"r": mtc, "m": runtime.SystemComponent} {
		if err := exec.SendMessage(port, runtime.NewInt("3"), nil, false); err != nil {
			t.Fatal(err)
		}
		if m, ok := exec.DequeueMessageFull(exec.PortKey(port)); !ok || m.Sender != want {
			t.Errorf("a send on %s: %v from %v, want one from %v", port, ok, m.Sender, want)
		}
	}
}

func sameKeys(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, k := range a {
		seen[k]++
	}
	for _, k := range b {
		if seen[k]--; seen[k] < 0 {
			return false
		}
	}
	return true
}
