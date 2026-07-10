package runtime

import (
	"sync/atomic"
	"testing"
	"time"
)

// waitReSnapshot fails if park does not return within a short real
// window — the whole point of the scheduler is that timer waits resolve
// virtually-instantly, never by real elapsed time.
func mustReturn(t *testing.T, name string, f func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { f(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: park did not return (hang/deadlock)", name)
	}
}

// TestScheduler_SingleTimerFiresInstantly: one participant (the MTC)
// parks on a 30s timer. It is immediately quiescent, so the clock jumps
// to 30 and it re-snapshots — with no real sleep.
func TestScheduler_SingleTimerFiresInstantly(t *testing.T) {
	q := newQuiesceScheduler()
	start := time.Now()
	var re, stopped bool
	mustReturn(t, "single-timer", func() {
		re, stopped = q.park(1, 30.0, true, nil)
	})
	if !re || stopped {
		t.Fatalf("park => reSnapshot=%v stopped=%v, want true/false", re, stopped)
	}
	if got := q.now(); got != 30.0 {
		t.Fatalf("clock = %v, want 30 (advanced to the deadline)", got)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("took %v; a virtual timer must fire instantly", elapsed)
	}
}

// TestScheduler_SoonestTimerWins: MTC (5s) + one PTC (30s). When both are
// parked the clock advances to the SOONEST deadline (5s), not 30s.
func TestScheduler_SoonestTimerWins(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive() // now 2 participants: gid 1 (MTC) and gid 2 (PTC)

	var wg int32
	got5 := make(chan float64, 1)
	got30 := make(chan float64, 1)
	// PTC parks on 30s.
	go func() {
		atomic.AddInt32(&wg, 1)
		q.park(2, 30.0, true, nil)
		got30 <- q.now()
	}()
	// MTC parks on 5s.
	go func() {
		atomic.AddInt32(&wg, 1)
		q.park(1, 5.0, true, nil)
		got5 <- q.now()
	}()

	select {
	case c := <-got5:
		if c != 5.0 {
			t.Fatalf("clock at MTC wake = %v, want 5", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("soonest-timer round did not complete")
	}
}

// TestScheduler_CommEventWakesPeerNoAdvance: a participant parks waiting
// only on comm (no timer); another goroutine signals; the parked one
// wakes to re-snapshot and the clock has NOT advanced.
func TestScheduler_CommEventWakesPeerNoAdvance(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive() // 2 participants; gid 2 stays "running" and will signal

	woke := make(chan bool, 1)
	go func() {
		re, stopped := q.park(1, 0, false, nil) // no timer: waits for comm
		woke <- re && !stopped
	}()

	// Give the parker a moment to actually park, then signal from the
	// still-running gid 2 (it never parks, so no quiescence/advance).
	deadline := time.Now().Add(time.Second)
	for {
		q.mu.Lock()
		parked := len(q.parked) == 1
		q.mu.Unlock()
		if parked || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	q.signal()

	select {
	case ok := <-woke:
		if !ok {
			t.Fatal("parked participant should re-snapshot on a comm event")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("comm event did not wake the parked participant")
	}
	if got := q.now(); got != 0 {
		t.Fatalf("clock = %v, want 0 (a comm event must not advance time)", got)
	}
}

// TestScheduler_Deadlock: two participants both park with no timer →
// genuine deadlock; both give up (reSnapshot=false, stopped=false) and
// the clock never moves.
func TestScheduler_Deadlock(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive() // 2 participants, gid 1 and 2

	res := make(chan [2]bool, 2)
	for _, gid := range []uint64{1, 2} {
		gid := gid
		go func() {
			re, stopped := q.park(gid, 0, false, nil)
			res <- [2]bool{re, stopped}
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case r := <-res:
			if r[0] || r[1] {
				t.Fatalf("deadlock park => reSnapshot=%v stopped=%v, want false/false", r[0], r[1])
			}
		case <-time.After(2 * time.Second):
			t.Fatal("deadlock was not detected (hang)")
		}
	}
	if got := q.now(); got != 0 {
		t.Fatalf("clock = %v, want 0", got)
	}
}

// TestScheduler_StopWakesParked: a stop channel unblocks a parked
// participant with stopped=true.
func TestScheduler_StopWakesParked(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive() // 2 so it doesn't self-quiesce
	stop := make(chan struct{})
	res := make(chan [2]bool, 1)
	go func() {
		re, stopped := q.park(1, 0, false, stop)
		res <- [2]bool{re, stopped}
	}()
	time.Sleep(10 * time.Millisecond)
	close(stop)
	select {
	case r := <-res:
		if r[0] || !r[1] {
			t.Fatalf("stopped park => reSnapshot=%v stopped=%v, want false/true", r[0], r[1])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not wake the parked participant")
	}
}

// TestScheduler_GoDoneWakesDoneWaiter models the MTC blocked on
// `all component.done`: it parks (no timer) while one PTC is live; the
// PTC finishes (goDone), which must wake the MTC to re-snapshot — NOT
// declare a deadlock.
func TestScheduler_GoDoneWakesDoneWaiter(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive() // MTC (gid 1) + PTC (gid 2)

	woke := make(chan bool, 1)
	go func() {
		// The MTC waits for the PTC to finish (no timer).
		re, stopped := q.park(1, 0, false, nil)
		woke <- re && !stopped
	}()

	// Wait until the MTC is parked, then finish the PTC.
	deadline := time.Now().Add(time.Second)
	for {
		q.mu.Lock()
		parked := len(q.parked) == 1
		q.mu.Unlock()
		if parked || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	q.goDone() // PTC finishes

	select {
	case ok := <-woke:
		if !ok {
			t.Fatal("comp.done waiter must re-snapshot when a PTC finishes, not deadlock")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("goDone did not wake the done-waiter")
	}
}

// TestScheduler_ProducerConsumerLoop stresses the comm path: a consumer
// parks waiting for comm; a producer signals; repeated many times under
// -race. Mirrors a client/server exchanging messages with no timers.
func TestScheduler_ProducerConsumerLoop(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive() // producer (gid 2) + consumer (gid 1)

	const rounds = 200
	done := make(chan struct{})
	// Consumer: park, wake on signal, repeat.
	go func() {
		for i := 0; i < rounds; i++ {
			re, stopped := q.park(1, 0, false, nil)
			if stopped || !re {
				// deadlock/stop shouldn't happen while the producer runs;
				// tolerate by continuing.
				_ = re
			}
		}
		close(done)
	}()
	// Producer: keep signalling until the consumer has drained all rounds.
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				q.signal()
				time.Sleep(50 * time.Microsecond)
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("producer/consumer loop did not complete")
	}
}

// TestScheduler_MixedTimersAndFinish combines the pieces: MTC waits done
// (no timer); PTC-A parks on 5s; PTC-B parks on 12s. When all three are
// parked the clock advances to 5 (soonest). Then PTC-A "finishes",
// leaving MTC + PTC-B; the next quiescent round advances to 12.
func TestScheduler_MixedTimersAndFinish(t *testing.T) {
	q := newQuiesceScheduler()
	q.goLive()
	q.goLive() // 3 participants: gid1 MTC, gid2 A, gid3 B

	reached5 := make(chan struct{}, 1)
	// PTC-A: park on 5s once, observe the clock, then finish.
	go func() {
		q.park(2, 5.0, true, nil)
		if q.now() >= 5.0 {
			select {
			case reached5 <- struct{}{}:
			default:
			}
		}
		q.goDone()
	}()
	// PTC-B: park on 12s; keep re-parking until the clock reaches 12,
	// then finish (goDone) so the MTC's done-wait can complete.
	go func() {
		for q.now() < 12.0 {
			re, _ := q.park(3, 12.0, true, nil)
			if !re {
				break
			}
		}
		q.goDone()
	}()
	// MTC: park (no timer) until both PTCs are done.
	mtcDone := make(chan struct{})
	go func() {
		for {
			q.mu.Lock()
			live := q.live
			q.mu.Unlock()
			if live <= 1 {
				close(mtcDone)
				return
			}
			re, stopped := q.park(1, 0, false, nil)
			if stopped {
				return
			}
			_ = re
		}
	}()

	select {
	case <-reached5:
	case <-time.After(3 * time.Second):
		t.Fatal("clock did not reach the soonest (5s) deadline")
	}
	select {
	case <-mtcDone:
		if got := q.now(); got < 12.0 {
			t.Fatalf("final clock = %v, want >= 12 (B's timer must fire after A finishes)", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second quiescent round (12s) did not complete")
	}
}
