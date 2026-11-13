package interpreter

// lifecycle.go centralises the component-state truth tables used by the
// `alive` / `running` / `done` / `killed` operations (ETSI ES 201 873-1
// clause 21.3) so the singular form (`c.done`), the array form
// (`any from carr.done`) and the all/any form (`all component.done`)
// answer consistently.

import (
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// floatSeconds converts a numeric runtime value (float or integer) to a
// float64 number of seconds.
func floatSeconds(v runtime.Object) (float64, bool) {
	switch n := v.(type) {
	case runtime.Float:
		return float64(n), true
	case runtime.Int:
		return float64(n.Int64()), true
	}
	return 0, false
}

// compStateStmt holds, per goroutine, the `.done` / `.killed` selector
// currently being evaluated as a statement. The selector's evaluator cannot
// see its own syntactic position, and the position decides the semantics:
// a statement blocks, an alt guard is a snapshot, and anywhere else the
// construct is not TTCN-3 at all. The node itself is recorded, not a flag,
// so the marker cannot reach a different selector evaluated beneath it.
var compStateStmt sync.Map // map[uint64]*syntax.SelectorExpr

// isCompStateOp reports whether sel is a `.done` or `.killed` operation.
func isCompStateOp(sel *syntax.SelectorExpr) bool {
	switch strings.ToLower(syntax.Name(sel.Sel)) {
	case "done", "killed":
		return true
	}
	return false
}

// evalCompStateStmt evaluates sel as the statement it appears as.
func evalCompStateStmt(sel *syntax.SelectorExpr, env runtime.Scope) runtime.Object {
	gid := goroutineID()
	prev, had := compStateStmt.Load(gid)
	compStateStmt.Store(gid, sel)
	defer func() {
		if had {
			compStateStmt.Store(gid, prev)
		} else {
			compStateStmt.Delete(gid)
		}
	}()
	return eval(sel, env)
}

// isCompStateStmt reports whether sel is the selector being evaluated as a
// statement on this goroutine.
func isCompStateStmt(sel *syntax.SelectorExpr) bool {
	v, ok := compStateStmt.Load(goroutineID())
	return ok && v.(*syntax.SelectorExpr) == sel
}

// compStateNotAValue is the error for `.done` / `.killed` used as a value,
// e.g. `if (c.done)`. It used to be answered — first by blocking and reading
// the result as false, later as a boolean computed after blocking, which a
// scheduler deadlock release could turn into `true` for a component still
// running. Neither answer is TTCN-3's, because the construct is not.
func compStateNotAValue(op string) runtime.Object {
	return runtime.Errorf("`.%s` is a statement or an alt guard, not a value (ETSI ES 201 873-1, "+
		"BNF 268/507); test a component's state in an expression with `.running` or `.alive`, "+
		"or check without blocking with `alt { [] c.%s {...} [else] {...} }`", op, op)
}

// blockUntilComponentState blocks the running participant until `ref`
// reaches the done / killed state (ETSI ES 201 873-1 §21.3.7/21.3.8: the
// standalone `comp.done` / `comp.killed` statements are blocking). Under
// the cooperative scheduler it PARKS — releasing the single-runner token —
// so the target PTC is granted the token and can actually run to
// completion; a non-parking check would let the MTC race ahead and starve
// the PTC (deadlock). A forked PTC is woken by its own goDone. Returns
// Undefined once the state holds or this participant is stopped.
func blockUntilComponentState(ref *runtime.ComponentRef, op string, env runtime.Scope) runtime.Object {
	res := waitComponentState(ref, op, env)
	if b, ok := res.(runtime.Bool); ok && bool(b) {
		tlDoneKilled(env, nil, op, "", ref)
	}
	return res
}

// waitComponentState performs blockUntilComponentState's wait.
func waitComponentState(ref *runtime.ComponentRef, op string, env runtime.Scope) runtime.Object {
	pred := compDone
	if op == "killed" {
		pred = compKilled
	}
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || ref == nil {
		return runtime.NewBool(pred(ref, env))
	}
	// In an interleave branch body, the interleave waits.
	for at := new(byte); !pred(ref, env) && interleaveWait(ilEvent{at: at, events: true}); {
	}
	stop := currentStopChan(exec)
	if !exec.SchedulerActive() {
		// Real clock: wait on the PTC's exit.
		waitPTCDoneRealClock(exec, []*runtime.ComponentRef{ref},
			func() bool { return pred(ref, env) }, stop, env, pred)
		return runtime.NewBool(pred(ref, env))
	}
	for !pred(ref, env) {
		re, stopped := exec.SchedPark(currentCompID(exec), 0, false, stop)
		if stopped || !re {
			break
		}
	}
	return runtime.NewBool(pred(ref, env))
}

// startOfKilled returns the error of starting a behaviour on ref, or of
// calling one, when ref was killed: a killed component runs no behaviour
// again. A non-alive one is killed with the end of its first behaviour,
// or its stop (ETSI 21.3.2, 21.3.3), so having had one is enough — its
// goroutine may not have recorded the kill yet when the starter, seeing
// it done, asks — and may still be unwinding, so nothing of the
// component is touched before this is asked.
func startOfKilled(exec *runtime.TestcaseExec, ref *runtime.ComponentRef, op string) runtime.Object {
	if exec == nil || ref == nil {
		return nil
	}
	prev := exec.PTCExit(ref.ID)
	if !ref.AliveModifier && prev != nil && !prev.Stopped() && !ref.IsDone() {
		return runtime.Errorf("%s of component %s, which is still running a behaviour (ETSI ES 201 873-1 21.3.2)", op, ref.Inspect())
	}
	if !ref.IsAlive() || (!ref.AliveModifier && prev != nil) {
		return runtime.Errorf("%s of component %s, which was killed (ETSI ES 201 873-1 21.3.2)", op, ref.Inspect())
	}
	return nil
}

// waitPreviousBehaviour waits until the behaviour last started on ref, if
// one was stopped and is still unwinding, has finished; on the cooperative
// scheduler it parks, so that behaviour can run to its end. Starting a
// component whose behaviour still runs, not stopped, is an error (ETSI
// 21.3.2).
func waitPreviousBehaviour(exec *runtime.TestcaseExec, ref *runtime.ComponentRef, env runtime.Scope) runtime.Object {
	prev := exec.PTCExit(ref.ID)
	if prev == nil {
		return nil
	}
	select {
	case <-prev.DoneChan:
		if !exec.SchedulerActive() {
			return nil
		}
	default:
		// Done but not yet unwound — its end seen by `c.done` — waits
		// like a stopped one.
		if !prev.Stopped() && !ref.IsDone() {
			return runtime.Errorf("start of component %s, which is still running a behaviour (ETSI ES 201 873-1 21.3.2)", ref.Inspect())
		}
	}
	stop := currentStopChan(exec)
	if exec.SchedulerActive() {
		exec.SchedParkWhileLive(currentCompID(exec), ref.ID, stop)
		return nil
	}
	select {
	case <-prev.DoneChan:
	case <-stop:
	}
	return nil
}

// blockUntilComponentsState is the `all component.done` / `any
// component.done` (and `.killed`) analogue of blockUntilComponentState: it
// parks the MTC on the cooperative scheduler until the aggregate predicate
// holds over the given started PTCs, so their forked bodies are granted the
// token and actually run (setting their verdict) rather than the MTC
// racing past a non-blocking snapshot. A forked PTC wakes the park when it
// finishes.
func blockUntilComponentsState(kind, op string, refs []*runtime.ComponentRef, env runtime.Scope) runtime.Object {
	res := waitComponentsState(kind, op, refs, env)
	if tlExec(env) != nil && componentsInState(kind, componentStatePredicate(op), refs, env) {
		tlDoneKilled(env, nil, op, kind, nil)
	}
	return res
}

// componentsInState reports whether the any/all predicate holds over refs.
func componentsInState(kind string, pred func(*runtime.ComponentRef, runtime.Scope) bool, refs []*runtime.ComponentRef, env runtime.Scope) bool {
	if kind == "any component" {
		for _, r := range refs {
			if pred(r, env) {
				return true
			}
		}
		return false
	}
	for _, r := range refs { // all component
		if !pred(r, env) {
			return false
		}
	}
	return true
}

// waitComponentsState performs blockUntilComponentsState's wait.
func waitComponentsState(kind, op string, refs []*runtime.ComponentRef, env runtime.Scope) runtime.Object {
	pred := compDone
	if op == "killed" {
		pred = compKilled
	}
	satisfied := func() bool { return componentsInState(kind, pred, refs, env) }
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return runtime.NewBool(satisfied())
	}
	// In an interleave branch body, the interleave waits.
	for at := new(byte); !satisfied() && interleaveWait(ilEvent{at: at, events: true}); {
	}
	stop := currentStopChan(exec)
	if !exec.SchedulerActive() {
		waitPTCDoneRealClock(exec, refs, satisfied, stop, env, pred)
		return runtime.NewBool(satisfied())
	}
	for !satisfied() {
		re, stopped := exec.SchedPark(currentCompID(exec), 0, false, stop)
		if stopped || !re {
			break
		}
	}
	return runtime.Undefined
}

// compAlive / compRunning / compDone / compKilled are the single source
// of truth for the four component-state predicates.
func compAlive(ref *runtime.ComponentRef, env runtime.Scope) bool {
	return componentState(ref, env).Alive
}

func compRunning(ref *runtime.ComponentRef, env runtime.Scope) bool {
	return componentState(ref, env).Running
}

func compDone(ref *runtime.ComponentRef, env runtime.Scope) bool {
	return componentState(ref, env).Done
}

func compKilled(ref *runtime.ComponentRef, env runtime.Scope) bool {
	return componentState(ref, env).Killed
}

// componentState is the state of ref as the component running in env
// sees it (see runtime.TestcaseExec.ComponentState).
func componentState(ref *runtime.ComponentRef, env runtime.Scope) runtime.ComponentState {
	exec := runtime.FindTestcaseExec(env)
	var by *runtime.ComponentRef
	if exec != nil && ref != nil && !ref.IsStarted() {
		by = exec.CurrentComponent()
	}
	return exec.ComponentState(ref, by)
}

// evalComponentDoneRedirect handles `comp.done -> value v` /
// `comp.killed -> value v`: it evaluates the done/killed predicate
// against the target component and, on a match, stores that component's
// local verdict into the redirect's value target. Returns (result,
// true) when the redirect wrapped a component done/killed selector;
// (nil, false) otherwise so the caller falls back to plain evaluation.
func evalComponentDoneRedirect(n *syntax.RedirectExpr, env runtime.Scope) (runtime.Object, bool) {
	if n == nil || n.X == nil {
		return nil, false
	}
	sel, ok := n.X.(*syntax.SelectorExpr)
	if !ok {
		return nil, false
	}
	op, ok := sel.Sel.(*syntax.Ident)
	if !ok {
		return nil, false
	}
	switch op.String() {
	case "done", "killed":
	default:
		return nil, false
	}
	recv := eval(sel.X, env)
	ref, ok := recv.(*runtime.ComponentRef)
	if !ok || ref == nil {
		return nil, false
	}
	// Outside an alt it is the statement `c.done -> value v;`, which waits
	// for the component as `c.done;` does (ETSI 21.3.7).
	if !altCtx.active() {
		if r := blockUntilComponentState(ref, op.String(), env); runtime.IsError(r) {
			return r, true
		}
	}
	var matched bool
	if op.String() == "done" {
		matched = compDone(ref, env)
	} else {
		matched = compKilled(ref, env)
	}
	if matched && len(n.Value) > 0 {
		v := ref.GetVerdict()
		if v == "" {
			v = runtime.NoneVerdict
		}
		storeReceiver(n.Value[0], v, env)
	}
	return runtime.NewBool(matched), true
}

// componentStatePredicate maps an op name to its predicate so the
// array / all-any query paths can share one switch.
func componentStatePredicate(op string) func(*runtime.ComponentRef, runtime.Scope) bool {
	switch op {
	case "alive":
		return compAlive
	case "running":
		return compRunning
	case "done":
		return compDone
	case "killed":
		return compKilled
	}
	return nil
}

// waitPTCDoneRealClock blocks until pred holds for every ref, on the real
// clock, where SchedPark returns immediately because there is no scheduler
// to park in. Without it `.done` answered a snapshot: the MTC ran on, the
// testcase ended, and teardown stopped PTCs that had not finished — so a
// PTC's `setverdict(fail)` was never reached and the testcase reported a
// pass nothing had earned.
//
// Each pass waits on everything that could change the answer, for every
// ref not yet in the state, and then re-checks the whole predicate — so
// `any component.done` returns when the FIRST of them finishes, and `all`
// when the last does. What a ref contributes depends on how it runs:
//   - a forked PTC: its exit (DoneChan closes when the goroutine ends);
//   - a PTC that has exited but is not in the state — an `alive` one whose
//     behaviour ended is done but not killed: nothing announces a later
//     kill, so a slow poll rather than a spin on the closed channel;
//   - a PTC with no behaviour running (never started, or started and
//     not yet registered): another component starting, stopping or
//     killing it, which wakes the waiters (WakeChan).
//
// The waiter's own stop signal and the testcase being stopped abort it.
func waitPTCDoneRealClock(exec *runtime.TestcaseExec, refs []*runtime.ComponentRef,
	done func() bool, stop <-chan struct{}, env runtime.Scope,
	pred func(*runtime.ComponentRef, runtime.Scope) bool) {

	defer markWaiting(exec)()
	markBlocked(exec)
	for {
		// Taken before the check, so a wake-up between the two is
		// not lost.
		wake := exec.WakeChan(currentCompID(exec))
		if done() {
			return
		}
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(stop)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(exec.StopChan())},
		}
		const aborts = 2
		var soonest time.Duration
		idle := false
		for _, r := range refs {
			if r == nil || pred(r, env) {
				continue
			}
			if exit := exec.PTCExit(r.ID); exit != nil {
				select {
				case <-exit.DoneChan:
					if soonest == 0 || realClockStatePoll < soonest {
						soonest = realClockStatePoll
					}
				default:
					cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(exit.DoneChan)})
				}
				continue
			}
			// No behaviour runs on it yet: it changes when another
			// component starts, stops or kills it, each of which wakes
			// the waiters (WakeChan). A started one whose behaviour is
			// not yet registered is in the same case.
			idle = true
		}
		var t *time.Timer
		if soonest > 0 {
			t = time.NewTimer(soonest)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(t.C)})
		}
		if idle {
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(wake)})
		}
		if len(cases) == aborts {
			return
		}
		chosen, _, _ := reflect.Select(cases)
		if t != nil {
			t.Stop()
		}
		if chosen < aborts {
			return
		}
	}
}

// realClockStatePoll is how often waitPTCDoneRealClock re-checks a PTC
// whose state can change without any event to wait on.
const realClockStatePoll = 5 * time.Millisecond
