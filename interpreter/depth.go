package interpreter

import (
	"sync"
	"sync/atomic"
)

// depthStore is a goroutine-local counter for tracking the current
// eval recursion depth. We need it per-goroutine because the
// conformance runner fans testcases out to a worker pool, and a
// process-wide counter would either race or charge one testcase's
// frames against another's budget.
//
// Implementation: a sync.Map of *uint64 counters keyed by goroutine
// id. enter()/leave() are the hot path -- every eval() pair hits
// them. The earlier mutex-protected map[uint64]int form serialised
// every goroutine onto a single lock and dominated interpreter
// runtime once a loop body started doing more than a handful of
// expressions; sync.Map plus an atomic counter takes the whole
// thing off the critical path.
type depthStore struct {
	counters sync.Map // map[uint64]*int64
}

// altCtx is a per-goroutine counter for "currently evaluating an
// alt-statement guard". When non-zero, standalone-mode behaviours
// like falling back to activated defaults should be suppressed - the
// alt scheduler handles guard mismatch on its own.
type altContext struct {
	counters sync.Map // map[uint64]*int64
}

var altCtx altContext

// altBodyCtx is set while the alt scheduler is evaluating the
// chosen clause's body (NOT the guard). The body is the only place
// the TTCN-3 21.2.4 `repeat` keyword may appear; without a body
// context the interpreter cannot tell a legitimate `repeat` from a
// stray one in a helper function and would have to silently
// promote every `repeat` to the runtime.Repeat sentinel.
var altBodyCtx altContext

// defaultCtx tracks whether the interpreter is currently inside a
// runDefaults() invocation. Reentry is suppressed so an activated
// altstep that itself ends up calling into the alt scheduler can't
// trigger a fresh defaults sweep, which would loop forever when no
// guard ever matches.
var defaultCtx altContext

func (a *altContext) counter(gid uint64) *int64 {
	if v, ok := a.counters.Load(gid); ok {
		return v.(*int64)
	}
	v, _ := a.counters.LoadOrStore(gid, new(int64))
	return v.(*int64)
}

func (a *altContext) enter() {
	atomic.AddInt64(a.counter(goroutineID()), 1)
}

func (a *altContext) leave() {
	gid := goroutineID()
	c := a.counter(gid)
	if atomic.AddInt64(c, -1) <= 0 {
		a.counters.Delete(gid)
	}
}

func (a *altContext) active() bool {
	if v, ok := a.counters.Load(goroutineID()); ok {
		return atomic.LoadInt64(v.(*int64)) > 0
	}
	return false
}

func (d *depthStore) counter(gid uint64) *int64 {
	if v, ok := d.counters.Load(gid); ok {
		return v.(*int64)
	}
	v, _ := d.counters.LoadOrStore(gid, new(int64))
	return v.(*int64)
}

func (d *depthStore) enter() bool {
	cur := atomic.AddInt64(d.counter(goroutineID()), 1)
	return cur <= int64(MaxEvalDepth)
}

func (d *depthStore) leave() {
	gid := goroutineID()
	c := d.counter(gid)
	if atomic.AddInt64(c, -1) <= 0 {
		d.counters.Delete(gid)
	}
}

// reset clears the depth for the current goroutine; the executor
// calls this at the start of every testcase so a leftover frame
// count from an earlier evaluation doesn't shrink the next budget.
func (d *depthStore) reset() {
	d.counters.Delete(goroutineID())
}

// goroutineID returns the current goroutine's runtime id. We read
// it directly from the runtime's g struct via the fast path in
// depth_runtime.go; see that file for the layout note. Falling back
// to runtime.Stack() parsing here would dominate interpreter cost
// (every eval() does one enter+leave -> two stack walks).
func goroutineID() uint64 {
	return fastGoroutineID()
}
