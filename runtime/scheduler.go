package runtime

// quiesceScheduler is a discrete-event "quiescence barrier" for the
// strict interpreter's deterministic clock. It replicates, at the
// application level, what Go's testing/synctest does in the runtime:
//
//	Virtual time advances ONLY when every live participant (the MTC and
//	all live PTC goroutines) is parked, and then it jumps to the soonest
//	registered timer deadline, waking the goroutine(s) whose timer fires.
//	A communication event (a message/call reaching a peer, or a peer
//	finishing) wakes parked participants at the CURRENT instant, before
//	any clock advance.
//
// This makes timer-driven concurrent execution fast (no real sleeps),
// sound (a safety timer can never fire before an earlier event), and free
// of the 2ms polling backstop's load-dependent flakiness.
//
// The type is deliberately self-contained (it owns the virtual clock and
// takes explicit goroutine ids) so it can be unit-tested in isolation
// with real goroutines under -race before it is wired into TestcaseExec.
// It is not yet referenced by the interpreter; enabling it is a separate,
// gated step.
//
// Concurrency: every field is guarded by mu. The wake channel is a
// broadcast — closing it wakes all parked goroutines; a fresh one is
// installed for the next round. A parked goroutine captures the current
// wake channel under the lock, releases the lock, then selects on it, so
// a broadcast that races the park is never lost.
//
// Deadlock is terminal and is decided ONLY on the parking path: it means
// every live participant is parked and none holds a timer, so no future
// event can occur (no participant is running to produce one). Because a
// running goroutine is required to call signal/goDone, neither can race a
// deadlock, so the flag never needs to be reset.

import "sync"

type parkEntry struct {
	deadline float64 // virtual-seconds deadline of this goroutine's soonest timer
	hasTimer bool    // false => waiting only on comm/component events (no timer)
}

type quiesceScheduler struct {
	mu       sync.Mutex
	clock    float64              // virtual time, seconds; monotonic
	live     int                  // live participants (MTC + PTC goroutines)
	parked   map[uint64]parkEntry // gid -> its registered wait
	wake     chan struct{}        // broadcast channel; closed+recreated per event
	deadlock bool                 // terminal: quiescent with no finite deadline
}

func newQuiesceScheduler() *quiesceScheduler {
	return &quiesceScheduler{
		parked: make(map[uint64]parkEntry),
		wake:   make(chan struct{}),
		live:   1, // the MTC (root) participant
	}
}

// now returns the current virtual time.
func (q *quiesceScheduler) now() float64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.clock
}

// advance moves the clock forward to `to` (never backward). Used for a
// single-participant timeout that fast-forwards to its own deadline
// without going through the barrier.
func (q *quiesceScheduler) advance(to float64) {
	q.mu.Lock()
	if to > q.clock {
		q.clock = to
	}
	q.mu.Unlock()
}

// goLive registers a newly forked participant. Call before the goroutine
// can park (i.e. at fork time, on the parent).
func (q *quiesceScheduler) goLive() {
	q.mu.Lock()
	q.live++
	q.mu.Unlock()
}

// goDone deregisters a finished participant. A finish is itself an event
// (it may satisfy a `comp.done` waiter), so it always wakes parked
// participants; if it leaves the system quiescent with a pending timer it
// also advances the clock so that timer still fires. It never declares a
// deadlock — a just-finished participant may be exactly what a parked
// waiter was blocked on, so the waiters get a chance to re-snapshot.
func (q *quiesceScheduler) goDone() {
	q.mu.Lock()
	if q.live > 0 {
		q.live--
	}
	if q.quiescentLocked() {
		q.advanceToSoonestLocked()
	}
	q.broadcastLocked()
	q.mu.Unlock()
}

// signal wakes all parked participants to re-snapshot. Call after a
// running participant produces an event a parked peer may be waiting on
// (a send/call/reply enqueued to a peer's queue, a stop, etc.). The
// caller is still running, so the system is not quiescent and the clock
// does not advance.
func (q *quiesceScheduler) signal() {
	q.mu.Lock()
	q.broadcastLocked()
	q.mu.Unlock()
}

// park blocks the calling participant (identified by gid) until it should
// re-snapshot. deadline/hasTimer describe its soonest timer guard (if
// any). stop wakes it on this participant's stop.
//
// Returns reSnapshot=true when a comm event arrived or the clock advanced
// (the caller re-evaluates its guards). Returns reSnapshot=false with
// stopped=true when stop fired, or reSnapshot=false with stopped=false on
// a genuine deadlock (quiescent, nothing left that can ever fire) — in
// both cases the caller concludes without matching.
func (q *quiesceScheduler) park(gid uint64, deadline float64, hasTimer bool, stop <-chan struct{}) (reSnapshot, stopped bool) {
	q.mu.Lock()
	q.parked[gid] = parkEntry{deadline: deadline, hasTimer: hasTimer}
	if q.quiescentLocked() {
		// This park completed a quiescent round. Advance to the soonest
		// timer; if none exists nobody can ever fire → terminal deadlock.
		if !q.advanceToSoonestLocked() {
			q.deadlock = true
		}
		dl := q.deadlock
		q.broadcastLocked()
		delete(q.parked, gid)
		q.mu.Unlock()
		return !dl, false
	}
	w := q.wake
	q.mu.Unlock()

	select {
	case <-w:
		// Woken by a broadcast: a comm event, a peer finishing, or a
		// clock advance from another goroutine completing a quiescent
		// round.
	case <-stop:
		q.mu.Lock()
		delete(q.parked, gid)
		q.mu.Unlock()
		return false, true
	}

	q.mu.Lock()
	delete(q.parked, gid)
	dl := q.deadlock
	q.mu.Unlock()
	return !dl, false
}

// quiescentLocked reports whether every live participant is parked. mu
// must be held.
func (q *quiesceScheduler) quiescentLocked() bool {
	return q.live > 0 && len(q.parked) >= q.live
}

// advanceToSoonestLocked moves the clock to the soonest finite deadline
// among parked participants and reports whether such a deadline existed.
// mu must be held.
func (q *quiesceScheduler) advanceToSoonestLocked() bool {
	var min float64
	have := false
	for _, p := range q.parked {
		if p.hasTimer && (!have || p.deadline < min) {
			min, have = p.deadline, true
		}
	}
	if have && min > q.clock {
		q.clock = min
	}
	return have
}

// broadcastLocked wakes every parked participant. mu must be held.
func (q *quiesceScheduler) broadcastLocked() {
	close(q.wake)
	q.wake = make(chan struct{})
}
