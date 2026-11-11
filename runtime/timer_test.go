package runtime_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/runtime"
)

// A timer started on the virtual clock expires at its deadline, reads the
// time elapsed, at most its duration, and reads 0 once expired or stopped
// (ETSI ES 201 873-1 23.2-23.7).
func TestTimerOnVirtualClock(t *testing.T) {
	at := func(v float64) runtime.TimerNow {
		return runtime.TimerNow{Virtual: v, HasVirtual: true, OnVirtual: true}
	}
	var th runtime.TimerHandle
	th.Start(at(1), 2)
	if th.VirtualDeadline() != 3 {
		t.Errorf("deadline = %g, want 3", th.VirtualDeadline())
	}
	if th.Expired(at(2.5)) || !th.Expired(at(3)) {
		t.Errorf("expired before or not at its deadline")
	}
	if got := th.Elapsed(at(2.5)); got != 1.5 {
		t.Errorf("elapsed = %g, want 1.5", got)
	}
	if got := th.Elapsed(at(4)); got != 0 {
		t.Errorf("elapsed after expiry = %g, want 0", got)
	}
	th.Fire()
	if th.Running || th.Expired(at(4)) || th.Poll(at(4)) {
		t.Errorf("a fired timer is still active")
	}
	th.Start(at(4), 1)
	th.Stop()
	if th.Running || th.Elapsed(at(4.5)) != 0 {
		t.Errorf("a stopped timer is still active")
	}
}

// On the virtual clock a timer polled with running more than
// TimerBusyWaitPolls times at one instant is taken to run no more; a new
// instant starts the count anew.
func TestTimerPollBusyWait(t *testing.T) {
	at := func(v float64) runtime.TimerNow {
		return runtime.TimerNow{Virtual: v, HasVirtual: true, OnVirtual: true}
	}
	var th runtime.TimerHandle
	th.Start(at(0), 5)
	for i := 0; i < runtime.TimerBusyWaitPolls; i++ {
		if !th.Poll(at(0)) {
			t.Fatalf("poll %d: not running", i)
		}
	}
	if th.Poll(at(0)) {
		t.Errorf("still running past the busy-wait limit")
	}
	if !th.Poll(at(1)) {
		t.Errorf("not running at a later instant")
	}
}

// On the wall clock a timer expires once its duration has passed, and one
// of no duration at once.
func TestTimerOnWallClock(t *testing.T) {
	start := time.Unix(1000, 0)
	at := func(d time.Duration) runtime.TimerNow { return runtime.TimerNow{Wall: start.Add(d)} }
	var th runtime.TimerHandle
	th.Start(at(0), 0.5)
	if th.Expired(at(400*time.Millisecond)) || !th.Expired(at(500*time.Millisecond)) {
		t.Errorf("expired before or not at its deadline")
	}
	if got := th.Elapsed(at(250 * time.Millisecond)); got != 0.25 {
		t.Errorf("elapsed = %g, want 0.25", got)
	}
	if !th.Poll(at(100 * time.Millisecond)) {
		t.Errorf("not running before its deadline")
	}
	th.Start(at(0), 0)
	if !th.Expired(at(0)) {
		t.Errorf("a timer of no duration did not expire at once")
	}
}
