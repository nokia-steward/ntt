package runtime_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// TestWakeChan: a message wakes the component whose port it is queued on
// — the MTC's ports are bare names, a PTC's qualified by its id — and no
// other; a stop wakes every component.
func TestWakeChan(t *testing.T) {
	exec := runtime.NewTestcaseExec("M.tc")
	exec.SetMTCID(1)
	mtc, ptc, idle := exec.WakeChan(1), exec.WakeChan(2), exec.WakeChan(3)

	exec.EnqueueMessage(exec.PortKeyFor(2, "p"), runtime.NewInt("1"))
	if !closed(ptc) || closed(mtc) || closed(idle) {
		t.Fatalf("a message to component 2: woke mtc=%v ptc=%v idle=%v", closed(mtc), closed(ptc), closed(idle))
	}
	exec.EnqueueMessage(exec.PortKeyFor(1, "p"), runtime.NewInt("2"))
	if !closed(mtc) || closed(idle) {
		t.Fatalf("a message to the MTC: woke mtc=%v idle=%v", closed(mtc), closed(idle))
	}
	again := exec.WakeChan(2)
	if closed(again) {
		t.Fatal("a woken component's next wait is woken already")
	}
	exec.Stop()
	if !closed(idle) || !closed(again) {
		t.Fatal("a stop did not wake every component")
	}
}
