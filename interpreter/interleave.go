package interpreter

// interleave.go runs `interleave` (ETSI ES 201 873-1 20.4): every branch
// is taken exactly once, and a branch body that reaches a receiving
// statement, an alt, a timeout or a done waits there while the other
// branches go on — the semantics the standard gives by expanding the
// interleave into nested alts. Each body that has started runs as a
// coroutine of the component: on a goroutine of its own, acting as the
// component, but only while the interleave hands it the turn. Where the
// component would wait, a body hands the turn back instead (interleaveWait),
// saying what it waits for; the interleave offers the branches not yet
// taken, resumes the bodies that wait, and when none of them can go on,
// waits as the component for the earliest of what they wait for.

import (
	"math"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// ilBranch is an interleave branch body running as a coroutine.
type ilBranch struct {
	resume chan bool    // the interleave hands the body the turn; false: give up
	yield  chan ilEvent // the body hands it back
	done   chan struct{} // the body's goroutine has ended
	at     interface{}  // where the body waits now (ilEvent.at)
	wait   ilEvent      // what it waits for
}

// ilEvent is what a body tells the interleave when it hands the turn back:
// that it has finished, with what result, or that it waits at `at` — for
// a virtual-clock deadline, a real one, or an event.
type ilEvent struct {
	finished bool
	res      runtime.Object
	panicked interface{}

	at       interface{} // this wait, told from any other
	deadline float64     // virtual
	hasTimer bool
	real     time.Duration // real clock: how long, at most
	hasReal  bool
	events   bool // a message or a component's end may end it too
}

// ilGiveUp unwinds a waiting body the interleave no longer resumes.
type ilGiveUp struct{}

// ilBranches maps a body's goroutine to its branch.
var ilBranches sync.Map // map[uint64]*ilBranch

func currentInterleaveBranch() *ilBranch {
	if v, ok := ilBranches.Load(goroutineID()); ok {
		return v.(*ilBranch)
	}
	return nil
}

// interleaveWait is called where the component would wait. In an
// interleave branch body it hands the turn back to the interleave, saying
// what the body waits for, and returns true once the body has the turn
// again, to look again. Anywhere else it returns false: wait as usual.
func interleaveWait(ev ilEvent) bool {
	br := currentInterleaveBranch()
	if br == nil {
		return false
	}
	br.yield <- ev
	if !<-br.resume {
		panic(ilGiveUp{})
	}
	return true
}

// startInterleaveBranch runs body as a coroutine of the current component
// until it first hands the turn back.
func startInterleaveBranch(body syntax.Node, env runtime.Scope) (*ilBranch, ilEvent) {
	br := &ilBranch{resume: make(chan bool), yield: make(chan ilEvent), done: make(chan struct{})}
	exec := runtime.FindTestcaseExec(env)
	var cur *runtime.ComponentRef
	if exec != nil {
		cur = exec.CurrentComponent()
	}
	go func() {
		defer close(br.done)
		gid := goroutineID()
		ilBranches.Store(gid, br)
		defer ilBranches.Delete(gid)
		if exec != nil && cur != nil {
			exec.PushComponent(cur)
			defer exec.PopComponent()
		}
		ev, gaveUp := runInterleaveBody(body, env)
		if !gaveUp {
			br.yield <- ev
		}
	}()
	return br, <-br.yield
}

func runInterleaveBody(body syntax.Node, env runtime.Scope) (ev ilEvent, gaveUp bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(ilGiveUp); ok {
				gaveUp = true
				return
			}
			ev = ilEvent{finished: true, panicked: r}
		}
	}()
	return ilEvent{finished: true, res: evalAltClauseBody(body, env)}, false
}

// next gives a waiting body the turn and returns what it says when it
// hands it back.
func (br *ilBranch) next() ilEvent {
	br.resume <- true
	return <-br.yield
}

// giveUp unwinds a waiting body, and returns once it has.
func (br *ilBranch) giveUp() {
	close(br.resume)
	<-br.done
}

// evalInterleaveStmtStrict evaluates `interleave { ... }` (ETSI 20.4):
// each round offers the branches not yet taken, in their order, and
// resumes each body that waits; a branch whose guard matches is taken and
// its body run until it finishes or waits. When nothing can go on, the
// activated defaults are tried (20.5) — one that fires leaves the
// interleave — and then the component waits for the earliest of what the
// branches wait for.
func evalInterleaveStmtStrict(n *syntax.AltStmt, env runtime.Scope) (result runtime.Object) {
	if n.Body == nil {
		return nil
	}
	var clauses []*syntax.CommClause
	for _, s := range n.Body.Stmts {
		if cc, ok := s.(*syntax.CommClause); ok && cc.Comm != nil && cc.Else == nil {
			clauses = append(clauses, cc)
		}
	}
	taken := make([]bool, len(clauses))
	running := make([]*ilBranch, len(clauses))
	remaining := len(clauses)
	defer func() {
		for _, br := range running {
			if br != nil {
				br.giveUp()
			}
		}
	}()
	// A body's turn ends: finished — leaving the interleave on break,
	// return, stop or an error — or waiting.
	settle := func(i int, br *ilBranch, ev ilEvent) (runtime.Object, bool, bool) {
		if !ev.finished {
			progressed := br.at != ev.at
			br.at, br.wait = ev.at, ev
			running[i] = br
			return nil, false, progressed
		}
		running[i] = nil
		remaining--
		if ev.panicked != nil {
			panic(ev.panicked)
		}
		// `repeat` is not permitted in interleave (20.4); ignore it.
		if ev.res != runtime.Repeat && needBreak(ev.res) {
			return ev.res, true, true
		}
		return nil, false, true
	}
	// Test logging: each branch taken begins a new round, and a scan after
	// waiting logs only what is new in its round (see evalAltStmtStrict).
	lexec := tlExec(env)
	if lexec != nil && defaultCtx.active() {
		lexec = nil
	}
	if lexec != nil {
		lexec.TLAltBump(currentCompID(lexec))
		defer lexec.TLScanEnd(currentCompID(lexec))
	}
	waiting := false
	waitAt := new(byte) // this interleave's wait, for one it is a body of
	const maxRounds = 1 << 20
	for round := 0; round < maxRounds && remaining > 0; round++ {
		// Alt-local declarations are re-evaluated each round (ETSI 20.2).
		for _, s := range n.Body.Stmts {
			if _, ok := s.(*syntax.CommClause); ok {
				continue
			}
			if r := eval(s, env); needBreak(r) {
				return r
			}
		}
		// The bodies that wait look again, in order.
		progressed := false
		for i, br := range running {
			if br == nil {
				continue
			}
			res, leave, moved := settle(i, br, br.next())
			if leave {
				return res
			}
			if moved {
				progressed = true
				break
			}
		}
		if progressed {
			waiting = false
			waitAt = new(byte) // it went on: an outer interleave sees it
			continue
		}
		// Freeze the visible-message boundary for the guard scan (ETSI 20.2
		// snapshot), same as evalAltStmtStrict: on the real-clock concurrent
		// path a message arriving mid-scan must not let a later branch take
		// what an earlier one would. Cleared before the matched body runs.
		altExec := runtime.FindTestcaseExec(env)
		freeze := altExec != nil && !deterministicSchedulerEnabled(env)
		if freeze {
			altExec.BeginAltRound(goroutineID())
		}
		if lexec != nil {
			lexec.TLScanBegin(currentCompID(lexec), waiting)
		}
		matchedIdx := -1
		for i, cc := range clauses {
			if taken[i] {
				continue
			}
			// Boolean guard (ETSI 20.2): eligible only when it holds.
			if cc.X != nil {
				if gv, ok := eval(cc.X, env).(runtime.Bool); ok && !bool(gv) {
					continue
				}
			}
			if commGuardMatches(cc.Comm, env) {
				matchedIdx = i
				defaultBranchFire() // no-op unless inside a runDefaults sweep
				break
			}
		}
		if freeze {
			altExec.EndAltRound(goroutineID())
		}
		if matchedIdx >= 0 {
			// An altstep is no alternative of an interleave (20.4); what
			// one taken would have left is not for a later alt.
			_ = takeAltstepResult()
			taken[matchedIdx] = true
			waiting = false
			if altExec != nil && altExec.TestLogger() != nil {
				altExec.TLScanEnd(currentCompID(altExec))
			}
			if lexec != nil {
				lexec.TLAltBump(currentCompID(lexec))
			}
			waitAt = new(byte)
			body := clauses[matchedIdx].Body
			if body == nil {
				remaining--
				continue
			}
			br, ev := startInterleaveBranch(body, env)
			if res, leave, _ := settle(matchedIdx, br, ev); leave {
				return res
			}
			continue // re-snapshot: taking one branch may enable another
		}
		// Nothing can go on. Activated defaults are appended after the
		// remaining alternatives (20.5); one that fires leaves the interleave.
		// An interleave in a branch body leaves them to the outer one.
		if !defaultsSuppressed(n) && currentInterleaveBranch() == nil {
			if ctl, fired := runDefaults(env); fired {
				if ctl == runtime.Repeat {
					// The default repeats: look again (20.5.2).
					if lexec != nil {
						tlEmit(lexec, n, "tliARepeat")
					}
					waiting = false
					waitAt = new(byte)
					continue
				}
				return ctl
			}
		}
		// This interleave IS the body of an activated default: a default is a
		// single non-blocking pass, so conclude instead of parking the token.
		if defaultCtx.active() {
			return nil
		}
		if lexec != nil {
			lexec.TLScanEnd(currentCompID(lexec))
		}
		waiting = true
		if !blockForInterleave(n, running, waitAt, env) {
			return nil
		}
	}
	return nil
}

// blockForInterleave waits, as the component, for the earliest of what
// the interleave's branches wait for: a guard of a branch not yet taken,
// or what a body waits for. Returns true to look again.
func blockForInterleave(n *syntax.AltStmt, running []*ilBranch, at interface{}, env runtime.Scope) bool {
	vd, hasVD := nextAltTimerVirtualDeadline(n, env)
	rd, hasRD := nextAltTimerDeadlineLenient(n, env)
	events := altHasEventGuard(n)
	for _, br := range running {
		if br == nil {
			continue
		}
		w := br.wait
		if w.hasTimer && (!hasVD || w.deadline < vd) {
			vd, hasVD = w.deadline, true
		}
		if w.hasReal && (!hasRD || w.real < rd) {
			rd, hasRD = w.real, true
		}
		if w.events || (!w.hasTimer && !w.hasReal) {
			events = true
		}
	}
	// An interleave in a branch body of another hands that one the turn.
	if interleaveWait(ilEvent{at: at, deadline: vd, hasTimer: hasVD, real: rd, hasReal: hasRD, events: events}) {
		return true
	}
	exec := runtime.FindTestcaseExec(env)
	if exec != nil && exec.Stopped() {
		return false
	}
	if exec != nil && exec.SchedulerActive() {
		if !hasVD && !events {
			return false
		}
		re, _ := exec.SchedPark(currentCompID(exec), vd, hasVD, currentStopChan(exec))
		return re
	}
	if deterministicClockEnabled(env) {
		if hasVD {
			advanceVirtual(env, vd)
			return true
		}
		if events {
			return waitForAltCombined(0, false, env)
		}
		return false
	}
	if hasRD || events {
		return waitForAltCombined(rd, hasRD, env)
	}
	return false
}

// timerWait is what a body waiting for timer th's timeout waits for: its
// deadline — none for a timer not running, whose timeout never comes.
func timerWait(th *runtime.TimerHandle, at interface{}) ilEvent {
	ev := ilEvent{at: at}
	if th == nil || !th.Running || th.Duration <= 0 {
		return ev
	}
	ev.deadline, ev.hasTimer = th.StartedAtVirtual+th.Duration, true
	if !th.StartedAt.IsZero() {
		left := time.Until(th.StartedAt.Add(time.Duration(th.Duration * float64(time.Second))))
		ev.real, ev.hasReal = time.Duration(math.Max(0, float64(left))), true
	}
	return ev
}
