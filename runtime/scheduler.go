package runtime

import "sync"

// coopScheduler is a deterministic cooperative scheduler for the strict
// interpreter. It replaces the broadcast quiescence barrier with a
// single-runner "token": exactly one component participant executes at a
// time, and the token is handed to the next participant in a
// deterministic order (lowest component id first). This removes the
// residual goroutine-interleaving nondeterminism of the plain quiescence
// model — with real goroutines running concurrently between park points,
// two components' snapshots could race (e.g. a client's getreply
// snapshot vs. a server's reply), making blocking-call verdicts flaky.
//
// Model:
//   - Every component (MTC + PTCs) is a "participant" keyed by its
//     component id (deterministic, unlike a goroutine id).
//   - The participant holding the token runs interpreter code until it
//     parks (blocks) or finishes; then it hands the token off.
//   - park() registers the participant's soonest timer deadline, releases
//     the token, and waits to be re-granted. Handoff picks the
//     lowest-id ready participant; if none is ready it advances the
//     virtual clock to the soonest deadline (quiescence) and grants that
//     participant; if nothing can ever fire it is a deadlock.
//   - A produced event (a send/call/reply, signalled via signal(); a
//     peer finishing, via goDone) marks parked participants ready so they
//     re-snapshot when granted.
//
// Virtual time advances only at quiescence, to the globally-soonest
// deadline — same clock discipline as before, but now interleaving is
// deterministic too. Self-contained and unit-tested in isolation before
// wiring into TestcaseExec.

type parkEntry struct {
	deadline float64 // virtual-seconds deadline of the soonest timer, if any
	hasTimer bool
}

type coopScheduler struct {
	mu       sync.Mutex
	clock    float64                 // virtual time, seconds; monotonic
	running  int64                   // participant id holding the token; 0 = none
	live     map[int64]bool          // live participants (MTC + PTCs)
	ready    map[int64]bool          // participants that can run now (forked or woken)
	blocked  map[int64]parkEntry     // parked participants + their deadlines
	turn     map[int64]chan struct{} // per-participant token-grant channel (buffered 1)
	deadlock bool                    // terminal: quiescent with no finite deadline
}

func newCoopScheduler(mtc int64) *coopScheduler {
	c := &coopScheduler{
		running: mtc, // the MTC holds the token from the start
		live:    map[int64]bool{mtc: true},
		ready:   map[int64]bool{},
		blocked: map[int64]parkEntry{},
		turn:    map[int64]chan struct{}{},
	}
	c.turn[mtc] = make(chan struct{}, 1)
	return c
}

// turnChLocked returns (creating if needed) the grant channel for id.
func (c *coopScheduler) turnChLocked(id int64) chan struct{} {
	ch, ok := c.turn[id]
	if !ok {
		ch = make(chan struct{}, 1)
		c.turn[id] = ch
	}
	return ch
}

// now returns the current virtual time.
func (c *coopScheduler) now() float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.clock
}

// advance moves the clock forward to `to` (never backward).
func (c *coopScheduler) advance(to float64) {
	c.mu.Lock()
	if to > c.clock {
		c.clock = to
	}
	c.mu.Unlock()
}

// goLive registers a newly forked participant. Called on the PARENT
// before the child goroutine starts, so the child is a schedulable
// candidate the moment its parent hands off the token. The child then
// calls acquireToken to actually take the token when granted.
func (c *coopScheduler) goLive(id int64) {
	c.mu.Lock()
	c.live[id] = true
	c.ready[id] = true
	c.turnChLocked(id)
	c.mu.Unlock()
}

// acquireToken blocks the calling (freshly forked) participant until it is
// granted the token. handoff sets running=id before granting, so on a
// false return the caller holds the token. `stop` (typically the PTC's
// stop channel, closed at teardown) unblocks a participant that was started
// but never scheduled — the MTC finished without ever parking, so this PTC
// was never granted a turn — returning true so the caller exits WITHOUT
// running its body. A nil stop channel never fires (blocks until granted).
func (c *coopScheduler) acquireToken(id int64, stop <-chan struct{}) (stopped bool) {
	c.mu.Lock()
	c.live[id] = true
	c.ready[id] = true
	w := c.turnChLocked(id)
	if c.running == 0 {
		c.handoffLocked()
	}
	c.mu.Unlock()
	select {
	case <-w:
		return false
	case <-stop:
		c.mu.Lock()
		delete(c.live, id)
		delete(c.ready, id)
		delete(c.blocked, id)
		// If a concurrent handoff granted us the token, drain it and pass
		// it on so the system doesn't stall on a participant giving up.
		select {
		case <-w:
		default:
		}
		if c.running == id {
			c.running = 0
			c.handoffLocked()
		}
		c.mu.Unlock()
		return true
	}
}

// goDone deregisters a finished participant (it held the token) and hands
// the token on. A finish is an event, so parked participants (e.g.
// comp.done waiters) are marked ready to re-snapshot.
func (c *coopScheduler) goDone(id int64) {
	c.mu.Lock()
	delete(c.live, id)
	delete(c.ready, id)
	delete(c.blocked, id)
	c.wakeAllBlockedLocked()
	if c.running == id {
		c.running = 0
		c.handoffLocked()
	}
	c.mu.Unlock()
}

// signal marks parked participants ready to re-snapshot after the running
// participant produced an event (a message/call/reply enqueued to a
// peer). The caller keeps the token; the handoff happens when it parks.
func (c *coopScheduler) signal() {
	c.mu.Lock()
	c.wakeAllBlockedLocked()
	// When every participant was parked there is no one holding the token
	// to hand it on, so the wake-up would go unnoticed. This is how a
	// stop reaches participants parked on an alt that can never fire.
	c.handoffLocked()
	c.mu.Unlock()
}

// wakeAllBlockedLocked moves every parked participant into the ready set
// so it re-snapshots on its next turn. Coarse but correct — a
// re-snapshot with no progress simply re-parks. mu held.
func (c *coopScheduler) wakeAllBlockedLocked() {
	for id := range c.blocked {
		c.ready[id] = true
		delete(c.blocked, id)
	}
}

// park releases the token, records the participant's soonest deadline,
// and blocks until it is granted the token again (a comm event woke it or
// the clock advanced to its timer) or `stop` fires. Returns reSnapshot=
// true to re-evaluate guards; reSnapshot=false with stopped=true on stop,
// or reSnapshot=false with stopped=false on a terminal deadlock.
func (c *coopScheduler) park(id int64, deadline float64, hasTimer bool, stop <-chan struct{}) (reSnapshot, stopped bool) {
	c.mu.Lock()
	delete(c.ready, id)
	c.blocked[id] = parkEntry{deadline: deadline, hasTimer: hasTimer}
	w := c.turnChLocked(id)
	if c.running == id {
		c.running = 0
	}
	c.handoffLocked()
	c.mu.Unlock()

	select {
	case <-w:
		c.mu.Lock()
		dl := c.deadlock
		c.running = id
		c.mu.Unlock()
		return !dl, false
	case <-stop:
		c.mu.Lock()
		delete(c.blocked, id)
		delete(c.ready, id)
		// If handoff granted us the token concurrently, release it so the
		// system doesn't stall on a participant that is giving up.
		select {
		case <-w:
		default:
		}
		if c.running == id {
			c.running = 0
			c.handoffLocked()
		}
		c.mu.Unlock()
		return false, true
	}
}

// handoffLocked grants the token to the next runnable participant. It is
// called with running==0. Order: (1) the lowest-id ready participant;
// (2) else, at quiescence, advance the clock to the soonest finite
// deadline and grant that participant; (3) else a terminal deadlock —
// release every parked participant so their alts conclude. mu held.
func (c *coopScheduler) handoffLocked() {
	if c.running != 0 {
		return
	}
	if id, ok := c.lowestReadyLocked(); ok {
		delete(c.ready, id)
		delete(c.blocked, id)
		c.running = id
		c.grantLocked(id)
		return
	}
	// No one is ready: quiescent. Advance to the soonest finite deadline.
	if id, dl, ok := c.soonestDeadlineLocked(); ok {
		if dl > c.clock {
			c.clock = dl
		}
		delete(c.blocked, id)
		c.running = id
		c.grantLocked(id)
		return
	}
	// Nothing can ever fire: every component is blocked and no timer can
	// advance the clock. That is a terminal deadlock, and in the loopback
	// model it is provable - no event can arrive from outside - so the
	// participants are released to unwind and TestcaseExec.SchedPark
	// turns it into an `error` verdict. Releasing without reporting is
	// what used to let a blocked alt conclude as though it had simply not
	// matched.
	if len(c.blocked) > 0 {
		c.deadlock = true
		for id := range c.blocked {
			delete(c.blocked, id)
			c.grantLocked(id)
		}
	}
}

func (c *coopScheduler) lowestReadyLocked() (int64, bool) {
	best := int64(0)
	found := false
	for id := range c.ready {
		if !found || id < best {
			best, found = id, true
		}
	}
	return best, found
}

func (c *coopScheduler) soonestDeadlineLocked() (int64, float64, bool) {
	var bestID int64
	var bestDL float64
	found := false
	for id, b := range c.blocked {
		if !b.hasTimer {
			continue
		}
		if !found || b.deadline < bestDL || (b.deadline == bestDL && id < bestID) {
			bestID, bestDL, found = id, b.deadline, true
		}
	}
	return bestID, bestDL, found
}

// grantLocked hands the token to id (non-blocking; the channel is
// buffered 1 and a not-yet-waiting participant consumes it on arrival).
func (c *coopScheduler) grantLocked(id int64) {
	ch := c.turnChLocked(id)
	select {
	case ch <- struct{}{}:
	default:
	}
}
