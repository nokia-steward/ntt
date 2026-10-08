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
	mu        sync.Mutex
	clock     float64                 // virtual time, seconds; monotonic
	running   int64                   // participant id holding the token; 0 = none
	live      map[int64]bool          // live participants (MTC + PTCs)
	ready     map[int64]bool          // participants that can run now (forked or woken)
	blocked   map[int64]parkEntry     // parked participants + their deadlines
	turn      map[int64]chan struct{} // per-participant token-grant channel (buffered 1)
	deadlock  bool                    // terminal: quiescent with no finite deadline
	drainer   int64                   // participant draining the others (see drain); 0 = none
	drainLeft int                     // turns the others may still take while draining
	drainSpin int                     // yields the others may still make while draining
	computing map[int64]bool          // ready because it yielded while computing, not woken
	spins     map[int64]int           // yields a participant made since it last waited
	instant   int                     // turns granted since the clock last moved
	woken     map[int64]parkEntry     // deadline a woken participant parked with, until it parks again
	passed    map[int64]int           // turns a ready participant was passed over for

	// limit bounds the testcase in virtual time (an execute() timeout):
	// the clock does not pass it; reaching it calls onLimit, once.
	limit    float64
	hasLimit bool
	limitHit bool
	onLimit  func()
}

// advanceLocked moves the clock to dl for a participant to be granted,
// and reports whether it may be: not past the testcase's limit. Reaching
// the limit ends the testcase instead (onLimit, off the lock). mu held.
func (c *coopScheduler) advanceLocked(dl float64) bool {
	if c.hasLimit && dl > c.limit {
		if c.limit > c.clock {
			c.clock = c.limit
		}
		if !c.limitHit {
			c.limitHit = true
			if c.onLimit != nil {
				go c.onLimit()
			}
		}
		return false
	}
	if dl > c.clock {
		c.clock = dl
		c.instant = 0
	}
	return true
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
		// Still live until it finishes (goDone): a starter may be waiting
		// for it, and must not take its absence for a deadlock.
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
	delete(c.computing, id)
	delete(c.spins, id)
	delete(c.woken, id)
	delete(c.ready, id)
	delete(c.blocked, id)
	c.wakeAllBlockedLocked()
	if c.running == id {
		c.running = 0
	}
	// A participant that finishes while unwinding from a stop does not
	// hold the token; with no one holding it, the woken ones would wait
	// for good.
	c.handoffLocked()
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
	for id, b := range c.blocked {
		c.ready[id] = true
		delete(c.blocked, id)
		if b.hasTimer {
			if c.woken == nil {
				c.woken = map[int64]parkEntry{}
			}
			c.woken[id] = b
		}
	}
}

// passTimeLocked lets computeQuantum of time pass for a computation, or an
// exchange, that goes on without waiting — less when a timer is due
// sooner: a parked participant's, or that of one woken and not yet run,
// whose timer must not be passed by before it looks. mu held.
func (c *coopScheduler) passTimeLocked() {
	to := c.clock + computeQuantum
	if _, dl, ok := c.soonestDeadlineLocked(); ok && dl < to {
		to = dl
	}
	for _, b := range c.woken {
		if b.deadline > c.clock && b.deadline < to {
			to = b.deadline
		}
	}
	c.advanceLocked(to)
}

// park releases the token, records the participant's soonest deadline,
// and blocks until it is granted the token again (a comm event woke it or
// the clock advanced to its timer) or `stop` fires. Returns reSnapshot=
// true to re-evaluate guards; reSnapshot=false with stopped=true on stop,
// or reSnapshot=false with stopped=false on a terminal deadlock.
func (c *coopScheduler) park(id int64, deadline float64, hasTimer bool, stop <-chan struct{}) (reSnapshot, stopped bool) {
	c.mu.Lock()
	delete(c.ready, id)
	delete(c.computing, id)
	delete(c.spins, id)
	delete(c.woken, id)
	delete(c.passed, id)
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

// parkWhileLive parks participant id until participant other is no longer
// live — until it has finished (goDone) — or stop fires, or, with
// hasDeadline, the clock reaches deadline. Whether other is live is
// checked under the scheduler's lock with the park itself, so a finish
// between a check and a park cannot be missed. Returns stopped on stop,
// deadlock on a terminal deadlock, timedOut when the deadline came first.
func (c *coopScheduler) parkWhileLive(id, other int64, deadline float64, hasDeadline bool, stop <-chan struct{}) (stopped, deadlock, timedOut bool) {
	for {
		c.mu.Lock()
		if !c.live[other] {
			c.mu.Unlock()
			return false, false, false
		}
		if hasDeadline && c.clock >= deadline {
			c.mu.Unlock()
			return false, false, true
		}
		delete(c.ready, id)
		delete(c.woken, id)
		c.blocked[id] = parkEntry{deadline: deadline, hasTimer: hasDeadline}
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
			if dl {
				return false, true, false
			}
		case <-stop:
			c.mu.Lock()
			delete(c.blocked, id)
			delete(c.ready, id)
			select {
			case <-w:
			default:
			}
			if c.running == id {
				c.running = 0
				c.handoffLocked()
			}
			c.mu.Unlock()
			return true, false, false
		}
	}
}

// drain lets the others run, at the end of the MTC's behaviour, until none
// can: each runs until it waits or finishes, a PTC started but never yet
// scheduled included, and then the token returns to id. The clock does not
// advance and nothing is a deadlock while draining — the drainer takes the
// token back when no one else can run. Components exchanging messages for
// good would never stop, so the drain ends after drainTurns turns; on the
// real clock the settle before teardown ends them the same way. Stop ends
// it too.
func (c *coopScheduler) drain(id int64, stop <-chan struct{}) {
	c.mu.Lock()
	c.drainLeft = drainTurns
	c.drainSpin = drainYields
	c.mu.Unlock()
	for {
		c.mu.Lock()
		if c.running != id || c.drainLeft <= 0 {
			c.mu.Unlock()
			return
		}
		others := false
		for r := range c.ready {
			if r != id {
				others = true
				break
			}
		}
		if !others {
			c.mu.Unlock()
			return
		}
		c.drainer = id
		delete(c.ready, id)
		delete(c.woken, id)
		c.blocked[id] = parkEntry{}
		w := c.turnChLocked(id)
		c.running = 0
		c.handoffLocked()
		c.mu.Unlock()
		select {
		case <-w:
			c.mu.Lock()
			c.running = id
			c.drainer = 0
			c.mu.Unlock()
		case <-stop:
			c.mu.Lock()
			c.drainer = 0
			delete(c.blocked, id)
			select {
			case <-w:
				c.running = id
			default:
			}
			c.mu.Unlock()
			return
		}
	}
}

// drainTurns bounds the turns the others take while the MTC drains them.
const drainTurns = 10000

// drainYields bounds the yields (see yield) the others make while the MTC
// drains them: about a tenth of a second of computing, as the real clock's
// settle before teardown allows.
const drainYields = 100

// computeYields is how many times a participant yields computing, without
// waiting, before time passes while it computes (see yield): about a
// million loop iterations; computeQuantum is the time that then passes.
const (
	computeYields  = 1000
	computeQuantum = 1.0 // seconds
)

// instantTurns is how many turns the scheduler grants at one instant of
// virtual time before time passes regardless (see handoffLocked).
const instantTurns = 100000

// yield hands the token on while participant id computes, and waits to get
// it back. On the virtual clock computing takes no time; a component that
// computes for long lets the others that can run now run — first those
// woken by an event, then the others computing, in turn — and time does not
// pass while it does. A component that computes for good would hold every
// timer still, though: after computeYields yields with no wait, however
// many others it let run, computeQuantum of time passes — less when a
// timer is due sooner, which then fires — as though that much computing
// took time. While the MTC drains the others, those
// computing get drainYields yields in all; then the drain ends, and the
// teardown's stop ends them. It returns at once when no one else is to run,
// and stopped on stop.
func (c *coopScheduler) yield(id int64, stop <-chan struct{}) (stopped bool) {
	c.mu.Lock()
	if c.running != id {
		c.mu.Unlock()
		return false
	}
	if c.spins == nil {
		c.spins = map[int64]int{}
	}
	c.spins[id]++
	cutOff := false
	if c.drainer != 0 {
		c.drainSpin--
		cutOff = c.drainSpin <= 0
	}
	// Computing for long without waiting takes time after all: a
	// quantum, up to the soonest deadline — which then fires — or the
	// testcase's limit — which then ends it.
	if !cutOff && c.drainer == 0 && c.spins[id] >= computeYields {
		c.spins[id] = 0
		c.passTimeLocked()
	}
	next, ok := c.wokenReadyLocked(id)
	if !ok && !cutOff {
		// One whose timer the clock has reached.
		if bid, dl, due := c.soonestDeadlineLocked(); due && dl <= c.clock {
			next, ok = bid, true
		}
	}
	if !ok {
		next, ok = c.nextComputingLocked(id)
	}
	if !ok && !cutOff {
		c.mu.Unlock()
		return false
	}
	w := c.turnChLocked(id)
	c.running = 0
	if cutOff {
		// The drain is over: the MTC takes the token back, and the
		// teardown stops the rest.
		c.drainLeft = 0
		delete(c.woken, id)
		c.blocked[id] = parkEntry{}
		c.handoffLocked()
	} else {
		c.ready[id] = true
		if c.computing == nil {
			c.computing = map[int64]bool{}
		}
		c.computing[id] = true
		delete(c.ready, next)
		delete(c.blocked, next)
		c.running = next
		c.grantLocked(next)
	}
	c.mu.Unlock()
	select {
	case <-w:
		c.mu.Lock()
		c.running = id
		c.mu.Unlock()
		return false
	case <-stop:
		c.mu.Lock()
		delete(c.ready, id)
		delete(c.blocked, id)
		select {
		case <-w:
			c.running = id
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

// wokenReadyLocked returns the lowest-id participant other than id ready
// because an event woke it or it was started, not because it yielded
// computing. mu held.
func (c *coopScheduler) wokenReadyLocked(id int64) (int64, bool) {
	woken, found := int64(0), false
	for r := range c.ready {
		if r != id && !c.computing[r] && (!found || r < woken) {
			woken, found = r, true
		}
	}
	return woken, found
}

// nextComputingLocked returns the participant computing next after id, in
// id order, wrapping round. mu held.
func (c *coopScheduler) nextComputingLocked(id int64) (int64, bool) {
	after, lowest, haveAfter, haveLowest := int64(0), int64(0), false, false
	for r := range c.ready {
		if r == id {
			continue
		}
		if r > id && (!haveAfter || r < after) {
			after, haveAfter = r, true
		}
		if !haveLowest || r < lowest {
			lowest, haveLowest = r, true
		}
	}
	if haveAfter {
		return after, true
	}
	return lowest, haveLowest
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
	// Draining (see drain): the others while they can run and have turns
	// left, then the drainer; the clock stays where it is.
	if d := c.drainer; d != 0 {
		if id, ok := c.lowestReadyLocked(); ok && c.drainLeft > 0 {
			c.drainLeft--
			delete(c.ready, id)
			delete(c.blocked, id)
			c.running = id
			c.grantLocked(id)
			return
		}
		if _, parked := c.blocked[d]; parked {
			delete(c.blocked, d)
			c.running = d
			c.grantLocked(d)
			return
		}
	}
	// Components that keep each other busy for good — two exchanging
	// messages, each waiting for the other's — never all wait, so the
	// clock would never move: after instantTurns turns at one instant,
	// computeQuantum of time passes, as for a long computation (see yield),
	// and a timer that comes due has the next turn.
	if len(c.ready) > 0 && c.drainer == 0 {
		c.instant++
		if c.instant >= instantTurns {
			c.instant = 0
			c.passTimeLocked()
			if id, dl, ok := c.soonestDeadlineLocked(); ok && dl <= c.clock {
				delete(c.blocked, id)
				c.running = id
				c.grantLocked(id)
				return
			}
		}
	}
	if id, ok := c.nextReadyLocked(); ok {
		delete(c.ready, id)
		delete(c.blocked, id)
		c.running = id
		c.grantLocked(id)
		return
	}
	// No one is ready: quiescent. Advance to the soonest finite deadline.
	if id, dl, ok := c.soonestDeadlineLocked(); ok {
		if !c.advanceLocked(dl) {
			return
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
	// A participant stopped while parked unwinds without the token: live,
	// but neither parked, ready nor running. Its finish (goDone) is an
	// event that wakes the parked ones, so while one is unwinding the
	// system is not deadlocked.
	for id := range c.live {
		if _, parked := c.blocked[id]; !parked && !c.ready[id] && id != c.running {
			return
		}
	}
	if len(c.blocked) > 0 {
		c.deadlock = true
		for id := range c.blocked {
			delete(c.blocked, id)
			c.grantLocked(id)
		}
	}
}

// nextReadyLocked picks the ready participant to run next: the lowest id,
// unless one has been passed over for starveTurns turns — two components
// keeping each other busy would otherwise keep a third, woken or started,
// from ever running — then the one passed over longest. mu held.
func (c *coopScheduler) nextReadyLocked() (int64, bool) {
	id, ok := c.lowestReadyLocked()
	if !ok {
		return 0, false
	}
	if c.passed == nil {
		c.passed = map[int64]int{}
	}
	most := 0
	for r := range c.ready {
		if n := c.passed[r]; n >= starveTurns && (n > most || (n == most && r < id)) {
			id, most = r, n
		}
	}
	for r := range c.ready {
		if r != id {
			c.passed[r]++
		}
	}
	delete(c.passed, id)
	return id, true
}

// starveTurns is how many turns a ready participant can be passed over for
// before it has the next (see nextReadyLocked).
const starveTurns = 1000

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
