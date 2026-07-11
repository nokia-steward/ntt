package runtime

import (
	"sync/atomic"
	"testing"
	"time"
)

func mustReturn(t *testing.T, name string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: did not return (hang/deadlock)", name)
	}
}

// TestCoop_SingleTimerInstant: the sole participant (MTC) parks on a 30s
// timer; it is immediately quiescent so the clock jumps to 30 and it is
// re-granted — no real wait.
func TestCoop_SingleTimerInstant(t *testing.T) {
	c := newCoopScheduler(1)
	start := time.Now()
	var re, stopped bool
	mustReturn(t, "single-timer", func() { re, stopped = c.park(1, 30, true, nil) })
	if !re || stopped {
		t.Fatalf("park => re=%v stopped=%v, want true/false", re, stopped)
	}
	if c.now() != 30 {
		t.Fatalf("clock=%v, want 30", c.now())
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("took %v; virtual timer must fire instantly", d)
	}
}

// TestCoop_TokenHandoff: MTC forks a PTC and parks; the PTC must be
// granted the token, run, and finish; the MTC then re-snapshots and sees
// the PTC done.
func TestCoop_TokenHandoff(t *testing.T) {
	c := newCoopScheduler(1)
	c.goLive(2)
	ptcRan := make(chan struct{})
	go func() {
		c.acquireToken(2) // waits for the token
		close(ptcRan)
		c.goDone(2)
	}()
	// MTC parks (no timer) → hands off to the PTC.
	mustReturn(t, "handoff", func() {
		re, _ := c.park(1, 0, false, nil) // woken when the PTC finishes
		_ = re
	})
	select {
	case <-ptcRan:
	case <-time.After(2 * time.Second):
		t.Fatal("PTC never got the token")
	}
}

// TestCoop_SoonestTimerWins: MTC (30s) + PTC (5s) both park; the clock
// advances to the soonest (5s) and the 5s participant is granted first.
func TestCoop_SoonestTimerWins(t *testing.T) {
	c := newCoopScheduler(1)
	c.goLive(2)
	got := make(chan float64, 2)
	go func() {
		c.acquireToken(2)
		c.park(2, 5, true, nil)
		got <- c.now()
		c.goDone(2)
	}()
	go func() {
		// let the PTC take the token and park first
		time.Sleep(20 * time.Millisecond)
		c.park(1, 30, true, nil)
		got <- c.now()
	}()
	select {
	case v := <-got:
		if v != 5 {
			t.Fatalf("first wake at clock=%v, want 5 (soonest)", v)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("soonest-timer round did not complete")
	}
}

// TestCoop_Deadlock: two participants park with no timer → terminal
// deadlock; both give up (re=false, stopped=false); clock unmoved.
func TestCoop_Deadlock(t *testing.T) {
	c := newCoopScheduler(1)
	c.goLive(2)
	res := make(chan [2]bool, 2)
	go func() {
		c.acquireToken(2)
		re, st := c.park(2, 0, false, nil)
		res <- [2]bool{re, st}
	}()
	go func() {
		time.Sleep(20 * time.Millisecond)
		re, st := c.park(1, 0, false, nil)
		res <- [2]bool{re, st}
	}()
	for i := 0; i < 2; i++ {
		select {
		case r := <-res:
			if r[0] || r[1] {
				t.Fatalf("deadlock park => re=%v stopped=%v, want false/false", r[0], r[1])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("deadlock not detected (hang)")
		}
	}
	if c.now() != 0 {
		t.Fatalf("clock=%v, want 0", c.now())
	}
}

// TestCoop_Stop: a stop channel unblocks a parked participant.
func TestCoop_Stop(t *testing.T) {
	c := newCoopScheduler(1)
	c.goLive(2) // so the MTC's park is not self-quiescent-deadlock
	stop := make(chan struct{})
	res := make(chan [2]bool, 1)
	go func() { res <- func() [2]bool { re, st := c.park(1, 0, false, stop); return [2]bool{re, st} }() }()
	time.Sleep(20 * time.Millisecond)
	close(stop)
	select {
	case r := <-res:
		if r[0] || !r[1] {
			t.Fatalf("stopped park => re=%v stopped=%v, want false/true", r[0], r[1])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not wake the parked participant")
	}
}

// TestCoop_MutualExclusion stresses the single-runner invariant: several
// participants repeatedly take the token and enter a critical section;
// the shared counter must never exceed 1 (only the token holder runs).
// Run under -race.
func TestCoop_MutualExclusion(t *testing.T) {
	c := newCoopScheduler(1)
	const nptc = 3
	const rounds = 200
	for id := int64(2); id < 2+nptc; id++ {
		c.goLive(id)
	}
	var inCritical int32
	var maxSeen int32
	done := make(chan struct{}, nptc+1)

	worker := func(id int64, isMTC bool) {
		if !isMTC {
			c.acquireToken(id)
		}
		for r := 0; r < rounds; r++ {
			// Critical section: only the token holder should be here.
			n := atomic.AddInt32(&inCritical, 1)
			for {
				m := atomic.LoadInt32(&maxSeen)
				if n <= m || atomic.CompareAndSwapInt32(&maxSeen, m, n) {
					break
				}
			}
			atomic.AddInt32(&inCritical, -1)
			// Produce an event so peers become ready, then park.
			c.signal()
			re, st := c.park(id, 0, false, nil)
			if st || !re {
				break // deadlock/stop: everyone stopped producing
			}
		}
		c.goDone(id)
		done <- struct{}{}
	}
	for id := int64(2); id < 2+nptc; id++ {
		go worker(id, false)
	}
	go worker(1, true)

	deadline := time.After(10 * time.Second)
	for i := 0; i < nptc+1; i++ {
		select {
		case <-done:
		case <-deadline:
			t.Fatal("mutual-exclusion stress did not complete")
		}
	}
	if maxSeen > 1 {
		t.Fatalf("two participants ran concurrently (maxSeen=%d); token broken", maxSeen)
	}
}
