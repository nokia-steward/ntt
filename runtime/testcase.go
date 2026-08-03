package runtime

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TestcaseExec is the per-testcase execution context carried by the
// interpreter while a testcase body runs. The interpreter installs it
// under the well-known env key TestcaseExecKey at the start of each
// testcase and looks it up from there when it needs to apply
// `setverdict` / `getverdict`, log a line, or check whether execution
// has been asked to stop.
//
// TestcaseExec is the interpreter-facing companion to runtime/report's
// types: the Verdict it carries is the same enum the report renderer
// emits, so once a testcase finishes the interpreter can hand the
// final verdict straight to the report layer without translation.
type TestcaseExec struct {
	Name string

	mu      sync.Mutex
	verdict Verdict
	reason  string
	log     []string
	stopped bool

	// mtcID is the component ID of the MTC, used by PortKey to keep the
	// MTC's ports on bare (unqualified) names so the single-MTC path is
	// byte-identical.
	mtcID int64

	// deterministicClock, when set, makes strict execution advance the
	// per-testcase virtual clock to a timer's deadline (firing it
	// instantly) instead of sleeping real wall-clock time. Timer expiry
	// then reads the virtual clock. This keeps timer-driven tests fast
	// and reproducible under the conformance harness (no real 5s waits,
	// no timeout artifacts); a real load driver leaves it off so timers
	// pace real I/O. Orthogonal to profile. Set once before PTCs fork.
	deterministicClock bool

	// sched, when non-nil, is the cooperative token scheduler (see
	// scheduler.go). It becomes the authority for virtual time and for
	// scheduling component goroutines: exactly one participant runs at a
	// time (single-runner token, deterministic handoff by component id),
	// and time advances only at quiescence to the soonest timer deadline.
	// This subsumes deterministicClock and makes concurrent timer-driven
	// execution deterministic (no real sleeps, no polling backstop, no
	// interleaving races). Set once before any PTC is started; nil on the
	// default/approximate path and for the real load driver.
	sched *coopScheduler

	// recvMu serializes the peek->match->dequeue->redirect critical
	// section of a port receive (see interpreter evalPortReceiveInfo).
	// Without it two PTC goroutines that share a port-instance name
	// (e.g. two daemon-style PTCs each declaring `port MyServer_PT
	// srv`) can both peek and match the same head message and then
	// dequeue two *different* messages, binding the `-> value`
	// redirect of both to the first one - so one request gets a
	// duplicate reply and another gets none. Distinct from mu so the
	// match-consume section (which calls back into the interpreter to
	// evaluate templates / redirects) never holds the map lock.
	recvMu sync.Mutex

	// ports maps a port-instance name to its FIFO of inbound
	// messages. We model loopback semantics: a `send` on port X
	// enqueues at the back of X's queue and a subsequent `receive`
	// on X dequeues from the front. This is the cheapest possible
	// stand-in for the runtime/port package and is enough for the
	// conformance suite, where the SUT-under-test is typically the
	// testcase itself echoing values back to itself.
	ports map[string][]PortMessage

	// componentTimers records every timer started on each component so
	// `any timer` / `all timer` (ETSI 23.7) can resolve the CURRENT
	// component's running timers DYNAMICALLY — a timer started in a testcase
	// body is visible to `any timer` inside a `runs on` altstep, which the
	// altstep's lexical env (chained to its module-level definition scope)
	// cannot reach. Keyed by component id; deduped by handle pointer. This is
	// the timer analogue of the exec-based dynamic port resolution.
	componentTimers map[int64][]*TimerHandle

	// deferredResponders are skipped PTC bodies that are waiting for a
	// procedure call to appear. The interpreter registers callbacks
	// here so runtime stays independent from the syntax package. Each
	// carries the responder component's ID so the strict scheduler can
	// replay only the responder(s) actually addressed by a call
	// (selective replay); the approximate path replays them all.
	deferredResponders []deferredResponder

	// portTypes maps a port-instance name (e.g. "cli") to its
	// declared port-type name (e.g. "MyClient_PT"). Populated
	// lazily when the interpreter walks a component-type body or
	// hits a port operation on an instance whose type hasn't been
	// recorded yet. Used by the PortDriver lookup to bind a C/C++
	// test port to a TTCN-3 port instance.
	portTypes map[string]string

	// boundPorts caches the result of the PortDriver lookup so the
	// hot send path stays a single map read. A nil entry means
	// "lookup performed, no driver bound"; a missing entry means
	// "lookup not yet attempted".
	boundPorts map[string]PortDriver
	driverMu   sync.Mutex

	// enc* are the per-execution encvalue round-trip caches. They
	// map an encoded blob back to the original value so a matching
	// decvalue / @decoded inside the SAME testcase can recover it.
	// Keeping them on the exec (rather than package globals) is
	// essential under the conformance harness, which runs many
	// testcases concurrently in one process: a value-identity key
	// (binValKey / string content) would otherwise collide across
	// unrelated testcases and hand back a foreign value.
	encMu     sync.Mutex
	encBinPtr map[*Binarystring]Object
	encBinVal map[string]Object
	encStrPtr map[*String]Object
	encStrVal map[string]Object

	// compMapped tracks which (local, remote) port pairs each
	// component currently has mapped. Indexed by ComponentRef.ID
	// so the runtime can drain (unmap) every entry when a PTC
	// exits / `d.stop` / `d.kill`.
	mappedMu   sync.Mutex
	compMapped map[int64]map[string]string

	// connGraph models the test-configuration topology for the
	// `checkstate` operation (ETSI 21.1.3): connections is the
	// connect/disconnect adjacency between (component,port)
	// endpoints, and portMapped marks endpoints that are currently
	// map'd to the system. Both are keyed by PortEndpoint so a
	// `p.checkstate("Connected"|"Mapped")` query answers from the
	// real edge state rather than a constant.
	connMu      sync.Mutex
	connections map[PortEndpoint]map[PortEndpoint]bool
	mapEdges    map[PortEndpoint]map[PortEndpoint]bool

	// portLifecycle records the explicit operational state of a port
	// endpoint set by `p.start` / `p.stop` / `p.halt` (ETSI 22.1).
	// A missing entry means the default "started" state, so a port
	// never touched by a lifecycle op keeps the optimistic
	// always-started checkstate answer.
	portLifecycle map[PortEndpoint]string

	// virtualClock is a monotonic per-testcase virtual time (in
	// seconds). It starts at 0 and is advanced by `T.timeout` to the
	// expiring timer's deadline so `T2.read` reports deterministic
	// virtual elapsed time independent of wall-clock jitter (ETSI
	// 23.4). Only timer `.read` consults it; `.running` still uses
	// the tick counter so spin loops terminate.
	virtualClock float64

	// defaults holds opaque ([]any-typed) altstep activations that
	// the interpreter should run when a standalone receive/check
	// fails to match. The runtime layer keeps them as plain
	// interface values so we don't have to import syntax / scope
	// types here; the interpreter casts them back at invocation
	// time.
	defaults      []Default
	nextDefaultId int

	// rnd is a per-testcase random source so parallel
	// conformance runs don't trample each other's `rnd(seed)`
	// state. Several Sem fixtures rely on `rnd(s); v := rnd();
	// rnd(s);` resetting the stream to the same point, which
	// only holds when no other goroutine seeded the global
	// math/rand between the two seeded calls.
	rndMu sync.Mutex
	rnd   *rand.Rand

	// nextCompID hands out monotonically increasing component
	// reference IDs so MyComp.create returns a ComponentRef the
	// `from <ref>` matcher can distinguish from any other ref the
	// same testcase produced.
	compMu     sync.Mutex
	nextCompID int64
	allComps   []*ComponentRef

	// compStack tracks the currently-running component for the
	// "main" execution thread (the testcase body or any
	// synchronous .start() invocation that didn't fork a PTC
	// goroutine). The per-goroutine PTC stacks live in
	// compStacks instead so async .start invocations don't trample
	// each other's sender tagging.
	compStack []*ComponentRef

	// compStacks holds one independent LIFO per PTC goroutine
	// (keyed by GoroutineIDFn() when it is set). When the
	// interpreter spawns f_body for `MyComp.create alive ; comp
	// .start(f)` in a goroutine, that goroutine pushes / pops
	// its component ref here, so `p.send(...)` in two PTCs
	// running concurrently can each pick up the right sender
	// without serialising on a single stack.
	compStacks sync.Map // map[uint64][]*ComponentRef

	// ptcExits tracks the cancel + done channels for every PTC
	// goroutine the interpreter spawned via an async .start on an
	// `alive` component. Keyed by ComponentRef.ID so the MTC can
	// look one up by name (`d.stop` -> StopPTC(d.ID)). Entries
	// stay around for the lifetime of the testcase so a second
	// `comp.stop` / `comp.kill` on the same ref is harmless.
	ptcMu    sync.Mutex
	ptcExits map[int64]*PTCExit

	// msgReady is a coalescing wake-up signal: every
	// EnqueueMessageFrom (loopback send + cabi inject path) does
	// a non-blocking send into it, and the alt-scheduler's
	// no-match path selects on it so a real-port-receive guard
	// that didn't match can park until new traffic arrives
	// instead of falling through to the verdict-preferring
	// legacy heuristic. Capacity 1 collapses bursts: between two
	// successful dequeues the alt sees at most one wake-up
	// regardless of how many sends actually happened, which is
	// all the level-triggered "retry the first pass" path needs.
	msgReadyOnce sync.Once
	msgReady     chan struct{}
}

// PTCExit is the per-PTC cancellation + completion envelope. The
// interpreter receives one from RegisterPTC() when it spawns a
// goroutine for `comp.start(f)` on an alive component, watches
// StopChan inside its alt scheduler / between statements, and
// signals DoneChan when the body finally returns so a later
// WaitPTCs() (testcase teardown) can join.
type PTCExit struct {
	StopChan chan struct{}
	DoneChan chan struct{}
	// MapChan closes the first time the PTC goroutine completes a
	// `map(self:p, system:p)` call. `comp.start` parks on it (with
	// a short fallback timeout) so the parent's next `d.start`
	// doesn't race ahead of the freshly forked daemon's initial
	// bind. Without this barrier, two PTCs that both `map(...)`
	// inside their body race for the cabi/cgo bridge's
	// pending_listens FIFO and a later `ds[i].stop` ends up
	// closing the wrong listener (the external C++ test-port assumes start-order = map-order
	// and pops front on each on_unmap).
	MapChan chan struct{}
	// SendChan closes the first time the PTC goroutine completes a
	// port `send(...)` through an external driver (the cabi/cgo
	// bridge). A daemon-style PTC binds its listen socket on that
	// first send (`srv.send(Bind{...})`), so `comp.start` parks on
	// SendChan after MapChan to guarantee the listener's bind()
	// syscall has been issued before the parent proceeds to query
	// it. Only consulted when a PortDriverProvider is installed, so
	// the loopback conformance path (no external driver) is
	// unaffected. Falls back to a timeout for receiver-only PTCs
	// that never send.
	SendChan chan struct{}
	stopOnce sync.Once
	doneOnce sync.Once
	mapOnce  sync.Once
	sendOnce sync.Once
}

// Stop closes StopChan exactly once. Safe to call from any
// goroutine. After Stop, a select on StopChan returns immediately,
// which is the signal the PTC body uses to unwind out of its
// blocking alt / receive / timer wait.
func (p *PTCExit) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() { close(p.StopChan) })
}

// Done closes DoneChan exactly once. Called by the PTC goroutine
// when its body returns so a later WaitPTCs can join cleanly.
func (p *PTCExit) Done() {
	if p == nil {
		return
	}
	p.doneOnce.Do(func() { close(p.DoneChan) })
}

// SignalMap closes MapChan exactly once. The interpreter calls it
// from the goroutine after a successful `map(...)` so the parent
// goroutine that forked this PTC can stop parking on the
// start-barrier. Safe (and idempotent) when MapChan was already
// signaled, or on Done() (which also closes MapChan as a fallback
// so a PTC body that never calls map doesn't leave the parent
// blocked).
func (p *PTCExit) SignalMap() {
	if p == nil {
		return
	}
	p.mapOnce.Do(func() {
		if p.MapChan != nil {
			close(p.MapChan)
		}
	})
}

// SignalSend closes SendChan exactly once. The interpreter calls it
// from the PTC goroutine after the first port send that went out
// through an external driver, so the parent's start-barrier knows the
// PTC's initial bind/connect has been issued. Idempotent and safe on
// a nil envelope or already-signaled channel (Done() closes it too as
// a fallback so a receiver-only PTC never dangles the parent).
func (p *PTCExit) SignalSend() {
	if p == nil {
		return
	}
	p.sendOnce.Do(func() {
		if p.SendChan != nil {
			close(p.SendChan)
		}
	})
}

// RegisterPTC creates and stores a fresh PTCExit envelope for the
// given component ref id. Returns the envelope so the interpreter
// goroutine can plumb StopChan into its alt-scheduler waits.
//
// Re-registering an id (e.g. a second `.start` on a re-startable
// alive component after a prior body returned) overwrites the
// previous envelope; the old StopChan / DoneChan are abandoned and
// any later Stop() on the stale envelope is a no-op.
func (t *TestcaseExec) RegisterPTC(refID int64) *PTCExit {
	p := &PTCExit{
		StopChan: make(chan struct{}),
		DoneChan: make(chan struct{}),
		MapChan:  make(chan struct{}),
		SendChan: make(chan struct{}),
	}
	t.ptcMu.Lock()
	if t.ptcExits == nil {
		t.ptcExits = map[int64]*PTCExit{}
	}
	t.ptcExits[refID] = p
	t.ptcMu.Unlock()
	return p
}

// PTCExit returns the cancel envelope associated with refID, or
// nil when the ref was never registered (synchronous .start path,
// MTC, or an unknown id).
// HasLivePTCs reports whether any forked PTC goroutine is still running
// (registered and not yet Done). The deterministic clock uses this to
// fall back to the real clock while concurrent PTCs are live — their
// events flow in real time, so advancing a virtual clock in one
// goroutine would race the others (a safety timer could fire before a
// peer's message/call arrives).
func (t *TestcaseExec) HasLivePTCs() bool {
	t.ptcMu.Lock()
	defer t.ptcMu.Unlock()
	for _, p := range t.ptcExits {
		select {
		case <-p.DoneChan:
			// finished; ignore
		default:
			return true
		}
	}
	return false
}

func (t *TestcaseExec) PTCExit(refID int64) *PTCExit {
	t.ptcMu.Lock()
	defer t.ptcMu.Unlock()
	return t.ptcExits[refID]
}

// StopPTC closes the matching PTCExit.StopChan and delivers a
// wake-up on MessageReady so a PTC goroutine parked in a blocked alt
// sees the cancel and unwinds. No-op when the id is unknown or has
// already been stopped.
func (t *TestcaseExec) StopPTC(refID int64) {
	t.ptcMu.Lock()
	p := t.ptcExits[refID]
	t.ptcMu.Unlock()
	if p == nil {
		return
	}
	p.Stop()
	t.signalMessageReady()
}

// FinishPTC closes the matching PTCExit.DoneChan from inside the
// PTC goroutine. No-op when the id is unknown. Also closes MapChan
// as a fallback so a PTC body that never calls `map(...)` doesn't
// leave the parent goroutine blocked on the start-barrier.
func (t *TestcaseExec) FinishPTC(refID int64) {
	t.ptcMu.Lock()
	p := t.ptcExits[refID]
	t.ptcMu.Unlock()
	if p != nil {
		p.SignalMap()
		p.SignalSend()
		p.Done()
	}
	// A finishing PTC deregisters from the scheduler (handing the token
	// on): it may satisfy a `comp.done` waiter and can make the system
	// quiescent.
	t.SchedGoDone(refID)
}

// WaitPTCs blocks until every registered PTC goroutine has signaled
// DoneChan, or until the global timeout expires. PTCs that miss
// the deadline are forcibly stopped (Stop() closes StopChan) and
// given a brief grace period to unwind before WaitPTCs returns
// regardless. Called from RunTestcaseWith on the way out so a
// PTC running an infinite alt doesn't dangle the testcase.
func (t *TestcaseExec) WaitPTCs(timeout time.Duration) {
	t.ptcMu.Lock()
	exits := make([]*PTCExit, 0, len(t.ptcExits))
	for _, p := range t.ptcExits {
		exits = append(exits, p)
	}
	t.ptcMu.Unlock()
	if len(exits) == 0 {
		return
	}
	// The MTC's testcase body has terminated; per ETSI ES 201 873-1 §21.3
	// every still-running PTC is implicitly stopped when the testcase ends.
	// Signal them ALL to unwind first, so a never-completing body
	// (`while(true){}`, a long timer someone forgot to stop) exits promptly
	// instead of holding teardown for the entire budget and pushing the run
	// past the harness timeout. A PTC that was about to finish on its own
	// still reports Done within the join budget below.
	for _, p := range exits {
		p.Stop()
	}
	deadline := time.Now().Add(timeout)
	for _, p := range exits {
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		select {
		case <-p.DoneChan:
		case <-time.After(remaining):
			select {
			case <-p.DoneChan:
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
}

// MessageReady returns the per-testcase wake-up channel that
// EnqueueMessageFrom signals on every enqueue. The alt scheduler
// reads it via a select{} after a no-match round so a real
// port-receive guard can park until traffic actually arrives.
// Capacity 1; the channel is created lazily so the existing
// NewTestcaseExec contract stays zero-config.
func (t *TestcaseExec) MessageReady() <-chan struct{} {
	t.msgReadyOnce.Do(func() {
		t.msgReady = make(chan struct{}, 1)
	})
	return t.msgReady
}

// signalMessageReady delivers one coalesced wake-up to the alt
// scheduler. Non-blocking: a buffer-full channel means an earlier
// enqueue already armed the wake-up and no scheduler has consumed
// it yet, so dropping the new signal is safe (the scheduler will
// re-enter the first pass and see the new message on its own).
func (t *TestcaseExec) signalMessageReady() {
	t.msgReadyOnce.Do(func() {
		t.msgReady = make(chan struct{}, 1)
	})
	select {
	case t.msgReady <- struct{}{}:
	default:
	}
	// Under the quiescence scheduler, waking parked peers is a broadcast
	// (the cap-1 msgReady bus above can only reliably wake one waiter).
	t.SchedSignal()
}

// PushComponent makes ref the currently-running component for the
// duration of a `.start(...)` call. Pair with PopComponent. When
// the caller is running on a PTC goroutine spawned by an async
// .start, the push goes into that goroutine's private compStacks
// lane so a sibling PTC running concurrently can pick up its own
// CurrentComponent.
func (t *TestcaseExec) PushComponent(ref *ComponentRef) {
	if gid, ok := callerGoroutineID(); ok {
		t.compStacks.Store(gid, append(loadCompStack(&t.compStacks, gid), ref))
		return
	}
	t.compMu.Lock()
	defer t.compMu.Unlock()
	t.compStack = append(t.compStack, ref)
}

// PopComponent reverses the most recent PushComponent on the
// calling goroutine's lane. Falls back to the shared compStack
// when the interpreter hasn't registered a GoroutineIDFn (e.g.
// runtime-only unit tests).
func (t *TestcaseExec) PopComponent() {
	if gid, ok := callerGoroutineID(); ok {
		stack := loadCompStack(&t.compStacks, gid)
		if n := len(stack); n > 0 {
			stack = stack[:n-1]
			if len(stack) == 0 {
				t.compStacks.Delete(gid)
			} else {
				t.compStacks.Store(gid, stack)
			}
		}
		return
	}
	t.compMu.Lock()
	defer t.compMu.Unlock()
	if n := len(t.compStack); n > 0 {
		t.compStack = t.compStack[:n-1]
	}
}

// CurrentComponent returns the component currently running on the
// calling goroutine's lane, or nil when nothing is pushed. Falls
// back to the shared compStack for non-PTC contexts (testcase body
// running on the main goroutine + the runtime-only unit tests).
func (t *TestcaseExec) CurrentComponent() *ComponentRef {
	if gid, ok := callerGoroutineID(); ok {
		if stack := loadCompStack(&t.compStacks, gid); len(stack) > 0 {
			return stack[len(stack)-1]
		}
	}
	t.compMu.Lock()
	defer t.compMu.Unlock()
	if n := len(t.compStack); n > 0 {
		return t.compStack[n-1]
	}
	return nil
}

func loadCompStack(m *sync.Map, gid uint64) []*ComponentRef {
	if v, ok := m.Load(gid); ok {
		return v.([]*ComponentRef)
	}
	return nil
}

// GoroutineIDFn returns the current goroutine's runtime id. It is
// set by the interpreter at init() time so the runtime package can
// key per-goroutine state (compStacks) without depending on the
// interpreter's fast-goid implementation. When nil, the per-lane
// component-stack path is skipped and PushComponent / PopComponent
// / CurrentComponent fall back to the shared compStack.
var GoroutineIDFn func() uint64

func callerGoroutineID() (uint64, bool) {
	if GoroutineIDFn == nil {
		return 0, false
	}
	return GoroutineIDFn(), true
}

// NewComponentID allocates a fresh ComponentRef id. The id is unique
// within the testcase; two refs with the same id refer to the same
// component instance.
func (t *TestcaseExec) NewComponentID() int64 {
	t.compMu.Lock()
	defer t.compMu.Unlock()
	t.nextCompID++
	return t.nextCompID
}

// RegisterComponent records ref in the per-testcase component
// registry so LatestComponentRef can hand it back to the alt-
// fallback heuristic later.
func (t *TestcaseExec) RegisterComponent(ref *ComponentRef) {
	t.compMu.Lock()
	defer t.compMu.Unlock()
	t.allComps = append(t.allComps, ref)
}

// LatestComponentRef returns the most recently created PTC ref, or
// nil when no components have been created yet. The alt-fallback
// heuristic uses this to populate `-> sender v` redirects when the
// real receive didn't fire.
func (t *TestcaseExec) LatestComponentRef() *ComponentRef {
	t.compMu.Lock()
	defer t.compMu.Unlock()
	if n := len(t.allComps); n > 0 {
		return t.allComps[n-1]
	}
	return nil
}

// AllComponents returns a snapshot of every component ref the
// testcase has created so far. Used by the `all component.<op>` /
// `any component.<op>` cross-component query path.
func (t *TestcaseExec) AllComponents() []*ComponentRef {
	t.compMu.Lock()
	defer t.compMu.Unlock()
	out := make([]*ComponentRef, len(t.allComps))
	copy(out, t.allComps)
	return out
}

// Rnd returns the next float in (0, 1) from the testcase-local random
// stream. If a seed has been provided via SeedRnd, the stream is
// deterministic from that point. Safe to call from any goroutine.
func (t *TestcaseExec) Rnd() float64 {
	t.rndMu.Lock()
	defer t.rndMu.Unlock()
	if t.rnd == nil {
		t.rnd = rand.New(rand.NewSource(1))
	}
	return t.rnd.Float64()
}

// SeedRnd reseeds the per-testcase random stream. Subsequent Rnd()
// calls produce the same sequence given the same seed (this is what
// the @lazy / @fuzzy default-parameter fixtures depend on).
func (t *TestcaseExec) SeedRnd(seed int64) {
	t.rndMu.Lock()
	defer t.rndMu.Unlock()
	t.rnd = rand.New(rand.NewSource(seed))
}

// Default is one activate() entry. Body is the altstep CallExpr
// (typed as Object so the runtime package stays free of syntax
// imports); Env is the activation-site scope so identifier lookups
// resolve where the user wrote `activate(a())`. Id is a monotonic
// handle the deactivate operation uses to remove the entry.
type Default struct {
	Id   int
	Body Object
	Env  Scope
}

// AddDefault registers an activated altstep and returns the handle
// that deactivate accepts as input.
func (t *TestcaseExec) AddDefault(d Default) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nextDefaultId++
	d.Id = t.nextDefaultId
	t.defaults = append(t.defaults, d)
	return d.Id
}

// RemoveDefault drops the activation with the given id, if any.
// Unknown ids are silently ignored (matching deactivate's semantics
// per TTCN-3 20.5.3 - an already-deactivated reference is harmless).
func (t *TestcaseExec) RemoveDefault(id int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, d := range t.defaults {
		if d.Id == id {
			t.defaults = append(t.defaults[:i], t.defaults[i+1:]...)
			return
		}
	}
}

// Defaults returns a copy of the activated-defaults stack. Safe to
// iterate while the testcase is running.
func (t *TestcaseExec) Defaults() []Default {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]Default, len(t.defaults))
	copy(out, t.defaults)
	return out
}

// ClearDefaults removes every activated default. It backs the bare
// `deactivate;` statement, which deactivates all defaults of the test
// component (ETSI 20.5.3).
func (t *TestcaseExec) ClearDefaults() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.defaults = nil
}

// PortMsgKind classifies a queued envelope so message receive and the
// procedure-based operations (call/getcall/reply/getreply/raise/catch,
// clause 22.3) can share a single port FIFO without consuming each
// other's entries. The zero value is MsgMessage, so every existing
// PortMessage{...} literal keeps its message semantics unchanged.
type PortMsgKind int

const (
	MsgMessage PortMsgKind = iota
	MsgCall
	MsgReply
	MsgException
)

// PortMessage is a single in-flight message carried by a loopback
// port queue. It bundles the payload with optional `to <addr>`
// metadata so `receive ... -> sender v` and `from addr` clauses can
// match on the same information the sender supplied.
type PortMessage struct {
	Payload Object
	Sender  Object // optional - nil if the send carried no `to` clause

	// Kind is the procedure-comm envelope class; the zero value
	// (MsgMessage) is an ordinary message. Signature carries the
	// procedure signature name for call/reply/exception envelopes
	// (empty for messages and when the op didn't name one).
	Kind      PortMsgKind
	Signature string

	// RetValue carries the procedure return value (reply) or the
	// exception value (raise). Payload holds the signature
	// parameter record for call/reply (so `-> param` and getcall/
	// getreply parameter templates match against it); RetValue is
	// what `-> value` binds and what the getreply `value <tmpl>`
	// clause / catch exception template match against.
	RetValue Object
}

// EnqueueMessage appends a payload to the named port's inbound queue.
// Threadsafe.
func (t *TestcaseExec) EnqueueMessage(port string, msg Object) {
	t.EnqueueMessageFrom(port, msg, nil)
}

// EnqueueMessageFrom is EnqueueMessage with an explicit `to <addr>`
// sender address. The address is whatever the interpreter evaluated
// the `to ...` clause to; it is not interpreted further here.
//
// After the queue is appended to, a non-blocking wake-up is sent
// on the testcase's MessageReady channel so any alt scheduler
// parked on an empty-queue real-port-receive guard can re-enter
// the first-pass walk. Capacity-1 channel: coalesces bursts.
func (t *TestcaseExec) EnqueueMessageFrom(port string, msg Object, sender Object) {
	t.mu.Lock()
	if t.ports == nil {
		t.ports = map[string][]PortMessage{}
	}
	t.ports[port] = append(t.ports[port], PortMessage{Payload: msg, Sender: sender})
	t.mu.Unlock()
	t.signalMessageReady()
}

// PeekMessage returns the head of the named port's queue without
// removing it. Returns (nil, false) when the queue is empty. Only the
// payload is returned; callers that need the sender address should
// use PeekMessageFull.
func (t *TestcaseExec) PeekMessage(port string) (Object, bool) {
	msg, ok := t.PeekMessageFull(port)
	if !ok {
		return nil, false
	}
	return msg.Payload, true
}

// PeekMessageFull is PeekMessage but returns the full envelope so
// callers can access the sender address.
func (t *TestcaseExec) PeekMessageFull(port string) (PortMessage, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// Skip procedure-comm envelopes (call/reply/exception): a
	// message receive only ever observes message-kind entries, so a
	// queued procedure call can never be consumed by `.receive`.
	for _, m := range t.ports[port] {
		if m.Kind == MsgMessage {
			return m, true
		}
	}
	return PortMessage{}, false
}

// PortNames lists every port the testcase has interacted with so far
// in deterministic (alphabetical) order. The interpreter uses this
// to drive `any port.receive(...)` style operations without having
// to track port declarations separately.
func (t *TestcaseExec) PortNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	names := make([]string, 0, len(t.ports))
	for k := range t.ports {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// DequeueMessage removes and returns the head of the named port's
// queue. Returns (nil, false) when the queue is empty.
func (t *TestcaseExec) DequeueMessage(port string) (Object, bool) {
	msg, ok := t.DequeueMessageFull(port)
	if !ok {
		return nil, false
	}
	return msg.Payload, true
}

// SetPortType records the declared port-type name for an instance
// (e.g. SetPortType("cli", "MyClient_PT")). Idempotent; subsequent
// calls with the same instance overwrite the previously-recorded
// type. The interpreter calls this when it walks a component-type
// body so the driver lookup can later resolve a C/C++ test port for
// the instance.
func (t *TestcaseExec) SetPortType(instance, typeName string) {
	if instance == "" || typeName == "" {
		return
	}
	t.mu.Lock()
	if t.portTypes == nil {
		t.portTypes = map[string]string{}
	}
	t.portTypes[instance] = typeName
	t.mu.Unlock()
}

// PortType returns the declared port-type name for an instance, or
// the empty string when no type was recorded.
func (t *TestcaseExec) PortType(instance string) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.portTypes[instance]
}

// PortDriver returns the bound external driver for a port instance,
// or nil when none is bound (the loopback case). Result is cached so
// repeated lookups on the hot path stay O(1).
//
// The provider hook (set via runtime.SetPortDriverProvider) is the
// indirection that lets the cabi/cgo bridge declare itself without
// the runtime package having to depend on cgo.
func (t *TestcaseExec) PortDriver(instance string) PortDriver {
	if !HasPortDriverProvider() {
		return nil
	}
	t.driverMu.Lock()
	if t.boundPorts == nil {
		t.boundPorts = map[string]PortDriver{}
	}
	if d, ok := t.boundPorts[instance]; ok {
		t.driverMu.Unlock()
		return d
	}
	t.driverMu.Unlock()

	typeName := t.PortType(instance)
	if typeName == "" {
		// A real-scheduler component-qualified key ("\x00c<id>/p")
		// won't have its own type binding — the type was recorded
		// under the bare name at component create — so resolve the
		// type via the bare name while keeping the qualified instance
		// for per-PTC driver identity.
		if bare := barePortName(instance); bare != instance {
			typeName = t.PortType(bare)
		}
	}
	if typeName == "" {
		// Without a type binding we still try the lookup with the
		// instance name itself - some test-port registrations use
		// the instance name as the registration key. Falls back to
		// nil silently when the lookup fails.
		typeName = instance
	}
	d := LookupPortDriver(typeName, instance)

	t.driverMu.Lock()
	t.boundPorts[instance] = d
	t.driverMu.Unlock()
	return d
}

// RecordPortMap remembers that component compID currently has port
// local mapped to remote. The pair is later drained by
// DrainPortMaps when the component exits / is stopped. Repeated calls overwrite the previous remote so a
// `map(self:p, system:p); map(self:p, system:q)` ends up with the
// most recent partner only.
func (t *TestcaseExec) RecordPortMap(compID int64, local, remote string) {
	if local == "" {
		return
	}
	t.mappedMu.Lock()
	defer t.mappedMu.Unlock()
	if t.compMapped == nil {
		t.compMapped = map[int64]map[string]string{}
	}
	m, ok := t.compMapped[compID]
	if !ok {
		m = map[string]string{}
		t.compMapped[compID] = m
	}
	m[local] = remote
}

// PortEndpoint identifies one end of a connection or mapping: a port
// instance (Port) on a particular test component (Comp, a
// ComponentRef.ID; the MTC / a system-side endpoint use a stable
// sentinel id). Used as the key of the test-configuration topology
// graph that `checkstate` queries.
type PortEndpoint struct {
	Comp int64
	Port string
}

// ConnectPorts records a bidirectional connection between two port
// endpoints (ETSI 21.1.1 connect). Idempotent.
func (t *TestcaseExec) ConnectPorts(a, b PortEndpoint) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.connections == nil {
		t.connections = map[PortEndpoint]map[PortEndpoint]bool{}
	}
	if t.connections[a] == nil {
		t.connections[a] = map[PortEndpoint]bool{}
	}
	if t.connections[b] == nil {
		t.connections[b] = map[PortEndpoint]bool{}
	}
	t.connections[a][b] = true
	t.connections[b][a] = true
}

// DisconnectPorts removes the connection between two endpoints (ETSI
// 21.1.2 disconnect). No-op when they were not connected.
func (t *TestcaseExec) DisconnectPorts(a, b PortEndpoint) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.connections == nil {
		return
	}
	if m := t.connections[a]; m != nil {
		delete(m, b)
	}
	if m := t.connections[b]; m != nil {
		delete(m, a)
	}
}

// DisconnectAll removes every connection touching endpoint a (the
// single-argument `disconnect(a:p)` / `disconnect` self form).
func (t *TestcaseExec) DisconnectAll(a PortEndpoint) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.connections == nil {
		return
	}
	for peer := range t.connections[a] {
		if m := t.connections[peer]; m != nil {
			delete(m, a)
		}
	}
	delete(t.connections, a)
}

// DisconnectComponent removes every connection of every port on the
// given component (the `disconnect(c:all port)` / `disconnect(c)` form,
// ETSI 21.1.2). Also clears the component's port mappings.
func (t *TestcaseExec) DisconnectComponent(compID int64) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.connections != nil {
		for ep := range t.connections {
			if ep.Comp != compID {
				continue
			}
			for peer := range t.connections[ep] {
				if m := t.connections[peer]; m != nil {
					delete(m, ep)
				}
			}
			delete(t.connections, ep)
		}
	}
	if t.mapEdges != nil {
		for ep := range t.mapEdges {
			if ep.Comp != compID {
				continue
			}
			for peer := range t.mapEdges[ep] {
				if m := t.mapEdges[peer]; m != nil {
					delete(m, ep)
				}
			}
			delete(t.mapEdges, ep)
		}
	}
}

// IsConnected reports whether endpoint a currently has at least one
// connection.
func (t *TestcaseExec) IsConnected(a PortEndpoint) bool {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	return len(t.connections[a]) > 0
}

// ConnectedToOther reports whether endpoint a is connected to a
// different endpoint (a true peer), as opposed to only a self-loop
// (`connect(self:p, self:p)`). A self-loop must deliver a component's
// own sent message back to itself, whereas a peer connection routes it
// to the peer - so the self-sent-message filter keys off this.
func (t *TestcaseExec) ConnectedToOther(a PortEndpoint) bool {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	for b := range t.connections[a] {
		if b != a {
			return true
		}
	}
	return false
}

// ConnectedPeers returns the endpoints currently connected to a
// (including a itself for a self-connect). Used to route a strict-profile
// send to the connected peer's port queue rather than the sender's own.
func (t *TestcaseExec) ConnectedPeers(a PortEndpoint) []PortEndpoint {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if len(t.connections[a]) == 0 {
		return nil
	}
	out := make([]PortEndpoint, 0, len(t.connections[a]))
	for b := range t.connections[a] {
		out = append(out, b)
	}
	return out
}

// PortKeyFor is PortKey for an explicit component id (rather than the
// current goroutine's component), used to address a connected peer's
// queue when routing a strict-profile send.
func (t *TestcaseExec) PortKeyFor(compID int64, name string) string {
	if name == "" || name == "any port" {
		return name
	}
	t.mu.Lock()
	mtc := t.mtcID
	t.mu.Unlock()
	if compID == mtc {
		return name
	}
	return portQualPrefix + strconv.FormatInt(compID, 10) + "/" + name
}

// SetPortLifecycle records the operational state ("started", "stopped"
// or "halted") of a port endpoint set by p.start / p.stop / p.halt
// (ETSI 22.1).
func (t *TestcaseExec) SetPortLifecycle(a PortEndpoint, state string) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.portLifecycle == nil {
		t.portLifecycle = map[PortEndpoint]string{}
	}
	t.portLifecycle[a] = state
}

// PortLifecycle returns the explicitly recorded operational state of a
// port endpoint and whether one was set. A missing entry means the
// default started state.
func (t *TestcaseExec) PortLifecycle(a PortEndpoint) (string, bool) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	s, ok := t.portLifecycle[a]
	return s, ok
}

// VirtualClock returns the current per-testcase virtual time (seconds).
// When the quiescence scheduler is active it owns the clock.
func (t *TestcaseExec) VirtualClock() float64 {
	if t.sched != nil {
		return t.sched.now()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.virtualClock
}

// AdvanceVirtualClock moves the virtual clock forward to `to` (never
// backward), modelling a `T.timeout` that fast-forwards to the timer's
// deadline. Returns the resulting virtual time.
func (t *TestcaseExec) AdvanceVirtualClock(to float64) float64 {
	if t.sched != nil {
		t.sched.advance(to)
		return t.sched.now()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if to > t.virtualClock {
		t.virtualClock = to
	}
	return t.virtualClock
}

// MapPorts records a bidirectional mapping edge between a component
// port and a system port (ETSI 21.1.1 map). Idempotent.
func (t *TestcaseExec) MapPorts(a, b PortEndpoint) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.mapEdges == nil {
		t.mapEdges = map[PortEndpoint]map[PortEndpoint]bool{}
	}
	if t.mapEdges[a] == nil {
		t.mapEdges[a] = map[PortEndpoint]bool{}
	}
	if t.mapEdges[b] == nil {
		t.mapEdges[b] = map[PortEndpoint]bool{}
	}
	t.mapEdges[a][b] = true
	t.mapEdges[b][a] = true
}

// UnmapPorts removes a specific mapping edge (`unmap(self:p,
// system:p)`).
func (t *TestcaseExec) UnmapPorts(a, b PortEndpoint) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.mapEdges == nil {
		return
	}
	if m := t.mapEdges[a]; m != nil {
		delete(m, b)
	}
	if m := t.mapEdges[b]; m != nil {
		delete(m, a)
	}
}

// UnmapAll removes every mapping edge touching endpoint a. Used by the
// single-argument `unmap(system:p)` / `unmap(self:p)` form, which
// unmaps every partner of the given side (ETSI 21.1.2).
func (t *TestcaseExec) UnmapAll(a PortEndpoint) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.mapEdges == nil {
		return
	}
	for peer := range t.mapEdges[a] {
		if m := t.mapEdges[peer]; m != nil {
			delete(m, a)
		}
	}
	delete(t.mapEdges, a)
}

// UnmapComponent removes every mapping edge of every port on the given
// component (the `unmap(c:all port)` form).
func (t *TestcaseExec) UnmapComponent(compID int64) {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	if t.mapEdges == nil {
		return
	}
	for ep := range t.mapEdges {
		if ep.Comp != compID {
			continue
		}
		for peer := range t.mapEdges[ep] {
			if m := t.mapEdges[peer]; m != nil {
				delete(m, ep)
			}
		}
		delete(t.mapEdges, ep)
	}
}

// ClearAllConnections / ClearAllMappings implement the
// `all component:all port` argument of disconnect / unmap (ETSI
// 21.1.2): release every connection / mapping in the configuration.
func (t *TestcaseExec) ClearAllConnections() {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	t.connections = nil
}

func (t *TestcaseExec) ClearAllMappings() {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	t.mapEdges = nil
}

// IsMapped reports whether endpoint a currently has at least one
// mapping edge.
func (t *TestcaseExec) IsMapped(a PortEndpoint) bool {
	t.connMu.Lock()
	defer t.connMu.Unlock()
	return len(t.mapEdges[a]) > 0
}

// ForgetPortMap drops the recorded (local, remote) entry for the
// given component, e.g. on an explicit `unmap(self:p, system:p)`.
// No-op when nothing was recorded.
func (t *TestcaseExec) ForgetPortMap(compID int64, local string) {
	t.mappedMu.Lock()
	defer t.mappedMu.Unlock()
	if t.compMapped == nil {
		return
	}
	if m, ok := t.compMapped[compID]; ok {
		delete(m, local)
		if len(m) == 0 {
			delete(t.compMapped, compID)
		}
	}
}

// DrainPortMaps returns and clears the list of (local, remote)
// port pairs the given component currently has mapped. Returns nil
// when the component never recorded any mapping. The caller is
// expected to walk the result and invoke driver.Unmap(local,
// remote) on each entry; doing that work inside the runtime would
// pull a cgo dependency in here, so we keep the callback up at the
// interpreter layer.
type PortMapEntry struct {
	Local  string
	Remote string
}

func (t *TestcaseExec) DrainPortMaps(compID int64) []PortMapEntry {
	t.mappedMu.Lock()
	defer t.mappedMu.Unlock()
	if t.compMapped == nil {
		return nil
	}
	m, ok := t.compMapped[compID]
	if !ok || len(m) == 0 {
		delete(t.compMapped, compID)
		return nil
	}
	out := make([]PortMapEntry, 0, len(m))
	for local, remote := range m {
		out = append(out, PortMapEntry{Local: local, Remote: remote})
	}
	delete(t.compMapped, compID)
	return out
}

// DrainAllPortMaps returns and clears the (local, remote) port pairs
// recorded across every component (MTC and all PTCs) in this
// testcase. The interpreter calls this once at teardown - after
// WaitPTCs has joined the PTC goroutines - so cabi/cgo C++ ports
// release their listen sockets before the next testcase starts.
// Without this, V2 tests that reuse the same daemon port numbers
// fail to bind ("socket error").
func (t *TestcaseExec) DrainAllPortMaps() []PortMapEntry {
	t.mappedMu.Lock()
	defer t.mappedMu.Unlock()
	if t.compMapped == nil {
		return nil
	}
	var out []PortMapEntry
	for compID, m := range t.compMapped {
		for local, remote := range m {
			out = append(out, PortMapEntry{Local: local, Remote: remote})
		}
		delete(t.compMapped, compID)
	}
	return out
}

// DequeueMessageFull is DequeueMessage but returns the full envelope.
// It removes and returns the first message-kind envelope, stepping
// over any interleaved procedure-comm envelopes so a `.receive` never
// dequeues a queued call/reply/exception.
func (t *TestcaseExec) DequeueMessageFull(port string) (PortMessage, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	q := t.ports[port]
	for i, m := range q {
		if m.Kind == MsgMessage {
			t.ports[port] = append(q[:i], q[i+1:]...)
			return m, true
		}
	}
	return PortMessage{}, false
}

// HasPendingCalls reports whether any port queue currently holds a
// procedure-call envelope. The interpreter uses it at `comp.start` to
// decide whether a finite responder body should run synchronously: a
// queued call means the caller already issued its nowait call(s)
// (pattern A) and the responder can consume them and reply.
func (t *TestcaseExec) HasPendingCalls() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, q := range t.ports {
		for _, m := range q {
			if m.Kind == MsgCall {
				return true
			}
		}
	}
	return false
}

// EnqueueEnvelope appends a full envelope (of any PortMsgKind) to the
// named port's queue and arms the alt wake-up, exactly like
// EnqueueMessageFrom does for messages. It is the enqueue entry point
// for the procedure-based operations (call / reply / raise).
func (t *TestcaseExec) EnqueueEnvelope(port string, msg PortMessage) {
	t.mu.Lock()
	if t.ports == nil {
		t.ports = map[string][]PortMessage{}
	}
	t.ports[port] = append(t.ports[port], msg)
	t.mu.Unlock()
	t.signalMessageReady()
}

// deferredResponder is one skipped PTC responder body plus the component it
// runs on, so RunDeferredResponders can (under the strict scheduler) replay
// only the responder addressed by a queued call.
type deferredResponder struct {
	compID int64
	fn     func()
}

// RegisterComponentTimer records that timer th was started on component
// compID, so `any timer` / `all timer` can later resolve the running timers
// of that component regardless of the lexical scope they are queried from
// (ETSI 23.7). Idempotent per handle pointer.
func (t *TestcaseExec) RegisterComponentTimer(compID int64, th *TimerHandle) {
	if th == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.componentTimers == nil {
		t.componentTimers = map[int64][]*TimerHandle{}
	}
	for _, x := range t.componentTimers[compID] {
		if x == th {
			return
		}
	}
	t.componentTimers[compID] = append(t.componentTimers[compID], th)
}

// CurrentComponentTimers returns the timers started on the calling
// goroutine's current component — the dynamic set `any timer` / `all timer`
// must consider. Empty when no component is current or none were started.
func (t *TestcaseExec) CurrentComponentTimers() []*TimerHandle {
	cur := t.CurrentComponent()
	if cur == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ts := t.componentTimers[cur.ID]
	if len(ts) == 0 {
		return nil
	}
	out := make([]*TimerHandle, len(ts))
	copy(out, ts)
	return out
}

// RegisterDeferredResponder records a skipped PTC procedure responder
// (running on component compID) that should be given another chance once a
// procedure call is queued.
func (t *TestcaseExec) RegisterDeferredResponder(compID int64, fn func()) {
	if fn == nil {
		return
	}
	t.mu.Lock()
	t.deferredResponders = append(t.deferredResponders, deferredResponder{compID: compID, fn: fn})
	t.mu.Unlock()
}

// RunDeferredResponders drains and invokes responders registered by
// RegisterDeferredResponder. Callbacks run outside the exec mutex because
// they evaluate TTCN-3 code and may enqueue/dequeue messages.
//
// Under the strict scheduler a responder is invoked ONLY when a call is
// actually queued on its component's ports — so a multicast `call to (...)`
// that addressed other components does not make this responder reply
// spuriously (Sem_220301_CallOperation_015). Un-addressed responders are
// re-registered for a later call.
func (t *TestcaseExec) RunDeferredResponders() {
	t.mu.Lock()
	drained := append([]deferredResponder{}, t.deferredResponders...)
	t.deferredResponders = nil
	t.mu.Unlock()

	var keep []deferredResponder
	for _, d := range drained {
		t.mu.Lock()
		addressed := t.hasPendingCallForCompLocked(d.compID)
		t.mu.Unlock()
		if !addressed {
			keep = append(keep, d)
			continue
		}
		d.fn()
	}
	if len(keep) > 0 {
		t.mu.Lock()
		t.deferredResponders = append(keep, t.deferredResponders...)
		t.mu.Unlock()
	}
}

// hasPendingCallForCompLocked reports whether any port queue belonging to
// component compID holds a queued MsgCall. Under the strict scheduler a
// non-MTC PTC's ports are qualified "\x00c<id>/<name>" (see PortKey), so we
// match that prefix. Caller holds t.mu.
func (t *TestcaseExec) hasPendingCallForCompLocked(compID int64) bool {
	prefix := portQualPrefix + strconv.FormatInt(compID, 10) + "/"
	for key, q := range t.ports {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		for _, m := range q {
			if m.Kind == MsgCall {
				return true
			}
		}
	}
	return false
}

// ReceiveLock acquires the receive critical-section mutex. The
// interpreter holds it for the whole peek->match->dequeue->redirect
// sequence of a port receive so concurrent PTC receivers on a shared
// port-instance name each claim a distinct message atomically.
func (t *TestcaseExec) ReceiveLock() { t.recvMu.Lock() }

// ReceiveUnlock releases the receive critical-section mutex.
func (t *TestcaseExec) ReceiveUnlock() { t.recvMu.Unlock() }

// PeekKind returns the first envelope of the given kind on the port
// without removing it, or (zero, false) when none is queued.
func (t *TestcaseExec) PeekKind(port string, kind PortMsgKind) (PortMessage, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, m := range t.ports[port] {
		if m.Kind == kind {
			return m, true
		}
	}
	return PortMessage{}, false
}

// DequeueKind removes and returns the first envelope of the given kind
// on the port, stepping over other-kind entries. Used by the
// procedure receive operations (getcall / getreply / catch).
func (t *TestcaseExec) DequeueKind(port string, kind PortMsgKind) (PortMessage, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	q := t.ports[port]
	for i, m := range q {
		if m.Kind == kind {
			t.ports[port] = append(q[:i], q[i+1:]...)
			return m, true
		}
	}
	return PortMessage{}, false
}

// TestcaseExecKey is the env-store key under which the interpreter
// stashes the active *TestcaseExec. The leading dunder makes it
// impossible to collide with a user-visible TTCN-3 identifier (TTCN-3
// identifiers cannot start with `_`).
const TestcaseExecKey = "__ntt_testcase_exec__"

// ModuleNameKey is the env-store key for the currently-executing
// module's name. The testcase runner sets this just before evaluating
// the body so __MODULE__ macro lookups can resolve it.
const ModuleNameKey = "__ntt_module_name__"

// ScopeNameKey is the env-store key for the innermost testcase /
// function name. Mirrors ModuleNameKey but for __SCOPE__.
const ScopeNameKey = "__ntt_scope_name__"

// NewTestcaseExec returns a fresh execution context with the verdict
// initialised to none.
func NewTestcaseExec(name string) *TestcaseExec {
	return &TestcaseExec{Name: name, verdict: NoneVerdict}
}

// RememberEnc records that binary blob bs (and any value-identity key
// valKey the caller computed for it) decodes back to v. See the enc*
// field comment for why this lives on the exec.
func (t *TestcaseExec) RememberEnc(bs *Binarystring, valKey string, v Object) {
	t.encMu.Lock()
	if t.encBinPtr == nil {
		t.encBinPtr = map[*Binarystring]Object{}
		t.encBinVal = map[string]Object{}
	}
	t.encBinPtr[bs] = v
	if valKey != "" {
		t.encBinVal[valKey] = v
	}
	t.encMu.Unlock()
}

// RecallEnc returns the original value for binary blob bs, trying the
// pointer identity first and the value-identity key second.
func (t *TestcaseExec) RecallEnc(bs *Binarystring, valKey string) (Object, bool) {
	t.encMu.Lock()
	defer t.encMu.Unlock()
	if v, ok := t.encBinPtr[bs]; ok {
		return v, true
	}
	if valKey != "" {
		if v, ok := t.encBinVal[valKey]; ok {
			return v, true
		}
	}
	return nil, false
}

// RememberEncStr / RecallEncStr are the string-blob counterparts of
// RememberEnc / RecallEnc (used by encvalue_unichar).
func (t *TestcaseExec) RememberEncStr(s *String, valKey string, v Object) {
	t.encMu.Lock()
	if t.encStrPtr == nil {
		t.encStrPtr = map[*String]Object{}
		t.encStrVal = map[string]Object{}
	}
	t.encStrPtr[s] = v
	t.encStrVal[valKey] = v
	t.encMu.Unlock()
}

func (t *TestcaseExec) RecallEncStr(s *String, valKey string) (Object, bool) {
	t.encMu.Lock()
	defer t.encMu.Unlock()
	if v, ok := t.encStrPtr[s]; ok {
		return v, true
	}
	if v, ok := t.encStrVal[valKey]; ok {
		return v, true
	}
	return nil, false
}

// currentExec stashes the testcase context the interpreter is
// currently running. The cabi/cgo bridge consults this via
// CurrentExec() when a C test port pushes an incoming payload through
// the runtime.inject() callback, so the bridge can route the message
// to the right TestcaseExec.EnqueueMessageFrom() destination without
// every port hook having to thread the exec through its parameter
// list. Single-slot because the interpreter runs testcases
// sequentially today; revisit if/when parallel testcase execution
// lands.
var currentExecMu sync.RWMutex
var currentExec *TestcaseExec

// SetCurrentExec records exec as the testcase context the runtime is
// currently executing. Pass nil to clear the binding (e.g. when a
// testcase finishes). Safe to call from any goroutine.
func SetCurrentExec(exec *TestcaseExec) {
	currentExecMu.Lock()
	currentExec = exec
	currentExecMu.Unlock()
}

// CurrentExec returns the testcase context the runtime is currently
// executing, or nil when no testcase is active. The cabi/cgo bridge
// uses this to push C-port-originated traffic back into the
// interpreter's port queue from a non-Go-stack thread.
func CurrentExec() *TestcaseExec {
	currentExecMu.RLock()
	defer currentExecMu.RUnlock()
	return currentExec
}

// componentTypePorts is the module-load-time registry of port-instance
// names to port-type names per component type. The interpreter walks
// each ComponentTypeDecl body once at module load and stashes the
// `<port-type> <port-instance>;` mappings here. When a testcase later
// runs `Comp.create`, newComponentRef copies the corresponding
// bindings onto the testcase exec so the cabi/cgo bridge's lookup of
// `port-type-name` (with instance-name fallback) finds the right
// driver. Without this registry the cabi/cgo lookup falls through to
// the loopback driver and the C-side port never sees traffic, which
// is a workaround some external test ports apply by registering
// each port under both its type and instance names.
var (
	componentTypePortsMu sync.RWMutex
	componentTypePorts   = map[string]map[string]string{}
)

// RegisterComponentTypePort records a `<portTypeName> <portInstance>`
// pair under componentTypeName. Safe to call from any goroutine;
// duplicate calls overwrite, which is intentional (a later
// module-load pass should see the freshest binding).
func RegisterComponentTypePort(componentTypeName, portInstance, portTypeName string) {
	if componentTypeName == "" || portInstance == "" || portTypeName == "" {
		return
	}
	componentTypePortsMu.Lock()
	defer componentTypePortsMu.Unlock()
	m, ok := componentTypePorts[componentTypeName]
	if !ok {
		m = map[string]string{}
		componentTypePorts[componentTypeName] = m
	}
	m[portInstance] = portTypeName
}

// ComponentTypePorts returns a copy of the registered port-instance ->
// port-type bindings for componentTypeName. Returns nil when no
// bindings are known. Callers receive a fresh map so callers may
// mutate it without affecting the registry.
func ComponentTypePorts(componentTypeName string) map[string]string {
	componentTypePortsMu.RLock()
	defer componentTypePortsMu.RUnlock()
	src := componentTypePorts[componentTypeName]
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// portTypeInstances is the testcase-local registry of port-type ->
// most-recently-mapped instance name. The cabi/cgo bridge registers
// the (type, instance) pair from its runtimeAdapter.Map; the inject
// dispatcher reads it back to route an incoming payload that the C
// side could only tag by registered port-type name (e.g.
// "MyClient_PT") to the right TTCN-3 instance queue (e.g. "cli").
// The map is cleared at testcase boundaries so a fresh exec sees an
// empty slate.
var (
	portTypeInstancesMu sync.RWMutex
	portTypeInstances   = map[string]string{}
)

// RegisterPortTypeInstance records that an instance of portTypeName
// is currently mapped under instanceName. The cabi/cgo bridge calls
// this from runtimeAdapter.Map so inject(<typeName>) can route back
// to the right TTCN-3 queue. Repeated calls overwrite; with multiple
// instances of one port type the most-recent wins.
func RegisterPortTypeInstance(portTypeName, instanceName string) {
	if portTypeName == "" || instanceName == "" {
		return
	}
	portTypeInstancesMu.Lock()
	defer portTypeInstancesMu.Unlock()
	portTypeInstances[portTypeName] = instanceName
}

// LookupPortTypeInstance returns the most-recently-mapped instance
// name for portTypeName, or "" when no instance has been recorded.
func LookupPortTypeInstance(portTypeName string) string {
	portTypeInstancesMu.RLock()
	defer portTypeInstancesMu.RUnlock()
	return portTypeInstances[portTypeName]
}

// ClearPortTypeInstances drops every (type -> instance) entry. Called
// at testcase teardown so the next testcase starts from a clean slate
// without inheriting bindings from a previous run.
func ClearPortTypeInstances() {
	portTypeInstancesMu.Lock()
	defer portTypeInstancesMu.Unlock()
	for k := range portTypeInstances {
		delete(portTypeInstances, k)
	}
}

// execTeardownHooks are invoked once per testcase run when the exec is
// torn down. A pure-Go port binding (goport) registers one to clear its
// package-global per-instance cache, so component IDs — which restart at
// each testcase — cannot alias a stale TestPort across runs. Kept in
// the runtime package so goport (which already imports runtime) can
// register without an import cycle.
var (
	execTeardownMu    sync.Mutex
	execTeardownHooks []func()
)

// RegisterExecTeardownHook adds fn to the set run at the end of every
// RunTestcaseWith. Idempotent registration is the caller's concern
// (goport installs its hook once, in Register).
func RegisterExecTeardownHook(fn func()) {
	if fn == nil {
		return
	}
	execTeardownMu.Lock()
	execTeardownHooks = append(execTeardownHooks, fn)
	execTeardownMu.Unlock()
}

// RunExecTeardownHooks invokes every registered teardown hook. Called
// from RunTestcaseWith's teardown defer.
func RunExecTeardownHooks() {
	execTeardownMu.Lock()
	hooks := append([]func(){}, execTeardownHooks...)
	execTeardownMu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

// SetVerdict applies TTCN-3 verdict aggregation: the resulting
// verdict is the max of the current and the new one by the canonical
// precedence none < pass < inconc < fail < error. `reason` is kept
// only the first time a fail or error is recorded so the message that
// flipped the testcase is preserved instead of a later, less
// informative one.
func (t *TestcaseExec) SetVerdict(v Verdict, reason string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if verdictRank(v) > verdictRank(t.verdict) {
		t.verdict = v
		if reason != "" && (v == FailVerdict || v == ErrorVerdict) {
			t.reason = reason
		}
	}
}

// GetVerdict returns the current verdict.
func (t *TestcaseExec) GetVerdict() Verdict {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.verdict
}

// Reason returns the message captured at the moment the verdict
// flipped to fail/error, if any.
func (t *TestcaseExec) Reason() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reason
}

// Log appends a line to the per-testcase log buffer. The interpreter
// calls Log on `log(...)` evaluations; tools render it as either a
// plain text stream or as JSON `output` events for the DAP / explorer
// IDE integrations.
func (t *TestcaseExec) Log(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.log = append(t.log, line)
}

// Logs returns a copy of the log buffer; safe to call while the
// testcase is still running.
func (t *TestcaseExec) Logs() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, len(t.log))
	copy(out, t.log)
	return out
}

// Stop marks the testcase as cancelled. The interpreter checks Stopped
// before each statement and unwinds the stack when it returns true.
// A wake-up is also delivered on MessageReady so any alt scheduler
// parked on an empty-queue real-port-receive guard sees the cancel
// without waiting out its poll deadline.
func (t *TestcaseExec) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
	t.signalMessageReady()
}

// Stopped reports whether the testcase has been asked to stop.
func (t *TestcaseExec) Stopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

// SetMTCID records the MTC's component ID so PortKey can leave the
// MTC's ports unqualified. Call once, right after the MTC ref exists.
func (t *TestcaseExec) SetMTCID(id int64) {
	t.mu.Lock()
	t.mtcID = id
	t.mu.Unlock()
}

// SetDeterministicClock enables/disables the deterministic (virtual)
// clock. Call once, before any PTC is started.
func (t *TestcaseExec) SetDeterministicClock(b bool) {
	t.mu.Lock()
	t.deterministicClock = b
	t.mu.Unlock()
}

// DeterministicClock reports whether timers advance the virtual clock
// instead of sleeping real wall-clock time.
func (t *TestcaseExec) DeterministicClock() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.deterministicClock
}

// SetDeterministicScheduler enables the discrete-event quiescence
// scheduler (see scheduler.go). Call once, before any PTC is started.
// Implies the deterministic clock. When enabled, component goroutines
// park through SchedPark instead of the real-clock waits, and virtual
// time advances only at quiescence.
func (t *TestcaseExec) SetDeterministicScheduler(b bool) {
	t.mu.Lock()
	if b && t.sched == nil {
		t.sched = newCoopScheduler(t.mtcID)
	} else if !b {
		t.sched = nil
	}
	t.mu.Unlock()
}

// SchedulerActive reports whether the cooperative scheduler owns timing.
func (t *TestcaseExec) SchedulerActive() bool {
	return t.sched != nil
}

// MTCID returns the MTC's component id (the scheduler's root participant).
func (t *TestcaseExec) MTCID() int64 {
	return t.mtcID
}

// SchedGoLive registers a newly forked component (by its ref id) with the
// scheduler. Call on the parent, before starting the goroutine, so the
// child is schedulable the moment the parent hands off the token. No-op
// when the scheduler is off.
func (t *TestcaseExec) SchedGoLive(id int64) {
	if t.sched != nil {
		t.sched.goLive(id)
	}
}

// SchedAcquireToken blocks a freshly forked component goroutine until the
// scheduler grants it the token. Call at the very start of the goroutine
// body. Returns true if `stop` fired first (the PTC was never scheduled and
// is being torn down) so the caller must exit without running its body.
// No-op returning false when the scheduler is off.
func (t *TestcaseExec) SchedAcquireToken(id int64, stop <-chan struct{}) bool {
	if t.sched != nil {
		return t.sched.acquireToken(id, stop)
	}
	return false
}

// SchedGoDone deregisters a finished component (by ref id), handing the
// token on. Called from FinishPTC. No-op when the scheduler is off.
func (t *TestcaseExec) SchedGoDone(id int64) {
	if t.sched != nil {
		t.sched.goDone(id)
	}
}

// SchedSignal marks parked participants ready to re-snapshot after an
// event a peer may be waiting on (a message/call enqueued, a stop). The
// caller keeps the token. No-op when the scheduler is off.
func (t *TestcaseExec) SchedSignal() {
	if t.sched != nil {
		t.sched.signal()
	}
}

// SchedPark releases the token and parks the given component until it
// should re-snapshot. deadline/hasTimer describe its soonest timer guard
// (0/false when it only waits on comm/component events). stop wakes it on
// this component's stop. Returns reSnapshot=true when a comm event
// arrived or the clock advanced; reSnapshot=false with stopped=true on
// stop, or reSnapshot=false with stopped=false on a terminal deadlock.
// Returns (false,false) immediately when the scheduler is off.
func (t *TestcaseExec) SchedPark(id int64, deadline float64, hasTimer bool, stop <-chan struct{}) (reSnapshot, stopped bool) {
	if t.sched == nil {
		return false, false
	}
	return t.sched.park(id, deadline, hasTimer, stop)
}

// portQualPrefix tags a component-qualified port key. The leading NUL
// cannot appear in a TTCN-3 identifier, so a qualified key never
// collides with a real port-instance name.
const portQualPrefix = "\x00c"

// PortKey returns a component-qualified port-instance key
// ("\x00c<compID>/<name>") so that N PTCs of the same type each get a
// PRIVATE port instance, queue, driver, and goport TestPort — the
// prerequisite for routing a Go port's Inject back to the specific
// worker that sent the request. It qualifies ONLY in real-scheduler
// mode and ONLY for a non-MTC PTC; the MTC and the default (flag-off)
// path keep bare names, so the single-MTC and conformance behaviours
// are byte-identical. `any port` is passed through so the aggregate
// receive ops keep resolving. Callers must invoke it on the PTC's own
// goroutine (where CurrentComponent() is that PTC), which is exactly
// where every send/receive/map runs.
func (t *TestcaseExec) PortKey(name string) string {
	if name == "" || name == "any port" {
		return name
	}
	t.mu.Lock()
	mtc := t.mtcID
	t.mu.Unlock()
	cur := t.CurrentComponent()
	if cur == nil || cur.ID == mtc {
		return name
	}
	return portQualPrefix + strconv.FormatInt(cur.ID, 10) + "/" + name
}

// CurrentComponentPortNames returns the bare (unqualified) port-instance
// names whose live queues belong to the calling goroutine's component —
// the set an `any port` operation on that component must consider (TTCN-3
// 22.5). Each returned name re-qualifies through PortKey (on this same
// goroutine) back to its storage key, so a caller can pass it straight to
// a per-port receive without double-qualifying. Ports owned by other
// components (a different qualifier) are excluded. The result is sorted
// (PortNames is), so any-port selection is deterministic. Must be called
// on the component's own goroutine, where CurrentComponent() is that
// component — exactly where receive ops run.
func (t *TestcaseExec) CurrentComponentPortNames() []string {
	var out []string
	for _, k := range t.PortNames() {
		bare := barePortName(k)
		if t.PortKey(bare) == k {
			out = append(out, bare)
		}
	}
	return out
}

// barePortName strips the qualifier PortKey added, returning the
// original TTCN-3 port-instance name; it is the identity for an
// unqualified name. Used where a bare-name lookup is still correct
// (e.g. resolving a port instance's declared type).
func barePortName(key string) string {
	if !strings.HasPrefix(key, portQualPrefix) {
		return key
	}
	if i := strings.IndexByte(key, '/'); i >= 0 {
		return key[i+1:]
	}
	return key
}

func (t *TestcaseExec) Type() ObjectType { return "testcase_exec" }
func (t *TestcaseExec) Inspect() string {
	return fmt.Sprintf("testcase(%q) verdict=%s logs=%d", t.Name, t.GetVerdict(), len(t.Logs()))
}
func (t *TestcaseExec) Equal(o Object) bool { return t == o }

// FindTestcaseExec walks an env chain looking for the active
// *TestcaseExec. Returns nil if no testcase context is in scope (for
// example when `setverdict` is called from the control part).
func FindTestcaseExec(s Scope) *TestcaseExec {
	if s == nil {
		return nil
	}
	if v, ok := s.Get(TestcaseExecKey); ok {
		if exec, ok := v.(*TestcaseExec); ok {
			return exec
		}
	}
	return nil
}

// ParseVerdict converts a TTCN-3 verdict identifier (case-insensitive)
// to its runtime Verdict value. Returns ("", false) on unknown input.
func ParseVerdict(s string) (Verdict, bool) {
	switch strings.ToLower(s) {
	case "none":
		return NoneVerdict, true
	case "pass":
		return PassVerdict, true
	case "inconc":
		return InconcVerdict, true
	case "fail":
		return FailVerdict, true
	case "error":
		return ErrorVerdict, true
	}
	return "", false
}

func verdictRank(v Verdict) int {
	switch v {
	case NoneVerdict:
		return 0
	case PassVerdict:
		return 1
	case InconcVerdict:
		return 2
	case FailVerdict:
		return 3
	case ErrorVerdict:
		return 4
	}
	return -1
}
