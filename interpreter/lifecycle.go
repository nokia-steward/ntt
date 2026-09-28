package interpreter

// lifecycle.go centralises the component-state truth tables used by the
// `alive` / `running` / `done` / `killed` operations (ETSI ES 201 873-1
// clause 21.3) so the singular form (`c.done`), the array form
// (`any from carr.done`) and the all/any form (`all component.done`)
// answer consistently.
//
// Beyond the explicit Alive/Done flags carried on a ComponentRef, a PTC
// started with a body that only blocks on a finite `timer.timeout` is
// modelled as terminating after that duration (componentCompleted): the
// MTC observes PTC liveness only after its own blocking timeout, which
// runs the real wall-clock forward, so comparing elapsed time against
// the modelled duration reproduces the relative ordering the tests rely
// on (the short-timer PTC is done/killed while the long-timer ones are
// still running) without a full virtual-time scheduler.

import (
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// modeledStartDuration inspects a `comp.start(body)` body and returns
// the wall-clock duration it would block for, when that is dominated by
// a finite `timer.timeout`. The body is typically a call `f(d)` whose
// function declares `timer t := <expr>; t.start; t.timeout`; we resolve
// f, bind its formals to the call's actuals, and evaluate the duration
// of the timer that is `.timeout`-ed (taking the longest when several).
// Returns (0, false) when no finite-timer model can be derived (an
// infinite loop, a port/proc wait, or a non-literal duration).
func modeledStartDuration(body syntax.Node, env runtime.Scope) (float64, bool) {
	root := body
	bodyEnv := env
	if ce, ok := body.(*syntax.CallExpr); ok {
		if id, ok := ce.Fun.(*syntax.Ident); ok {
			if v, ok := env.Get(id.String()); ok {
				if fn, ok := forceThunk(v).(*runtime.Function); ok && fn.Body != nil {
					var args []runtime.Object
					if ce.Args != nil {
						for _, a := range ce.Args.List {
							args = append(args, eval(a, env))
						}
					}
					fenv := runtime.NewEnv(fn.Env)
					bindParamsInto(fenv, fn.Params, args)
					root = fn.Body
					bodyEnv = fenv
				}
			}
		}
	}

	durations := map[string]syntax.Expr{}
	var timeoutTimers []string
	hasWhileTrue := false
	syntax.Inspect(root, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			// A timer declaration is `timer t := <dur>;`: the `timer`
			// keyword is the declaration's Type (an Ident whose token
			// is TIMER), not the KindTok (which carries VAR/CONST/...).
			isTimer := x.KindTok != nil && x.KindTok.Kind() == syntax.TIMER
			if id, ok := x.Type.(*syntax.Ident); ok && id.Tok.Kind() == syntax.TIMER {
				isTimer = true
			}
			if isTimer {
				for _, d := range x.Decls {
					if d != nil && d.Name != nil && d.Value != nil {
						durations[d.Name.String()] = d.Value
					}
				}
			}
		case *syntax.SelectorExpr:
			if id, ok := x.Sel.(*syntax.Ident); ok && id.String() == "timeout" {
				if tid, ok := x.X.(*syntax.Ident); ok {
					timeoutTimers = append(timeoutTimers, tid.String())
				}
			}
		case *syntax.WhileStmt:
			if lit, ok := x.Cond.(*syntax.ValueLiteral); ok && lit.Tok.Kind() == syntax.TRUE {
				hasWhileTrue = true
			}
		}
		return true
	})
	if hasWhileTrue || len(timeoutTimers) == 0 {
		return 0, false
	}

	best := 0.0
	found := false
	for _, name := range timeoutTimers {
		de, ok := durations[name]
		if !ok {
			continue
		}
		if v := eval(de, bodyEnv); !runtime.IsError(v) {
			if f, ok := floatSeconds(v); ok {
				found = true
				if f > best {
					best = f
				}
			}
		}
	}
	if !found || best <= 0 {
		return 0, false
	}
	return best, true
}

// modeledBodyKills reports whether a `comp.start(body)` body terminates
// with a `kill` (a bare `kill;`, `self.kill`, or `mtc.kill`). It is used
// when the body is skipped so the modelled-completion path can mark the
// component killed rather than merely done (ETSI 21.3.4/21.3.8).
func modeledBodyKills(body syntax.Node, env runtime.Scope) bool {
	root := body
	if ce, ok := body.(*syntax.CallExpr); ok {
		if id, ok := ce.Fun.(*syntax.Ident); ok {
			if v, ok := env.Get(id.String()); ok {
				if fn, ok := forceThunk(v).(*runtime.Function); ok && fn.Body != nil {
					root = fn.Body
				}
			}
		}
	}
	kills := false
	syntax.Inspect(root, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Ident:
			if x.String() == "kill" {
				kills = true
			}
		case *syntax.SelectorExpr:
			if id, ok := x.Sel.(*syntax.Ident); ok && id.String() == "kill" {
				kills = true
			}
		}
		return true
	})
	return kills
}

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

// componentCompleted reports whether a modelled finite-timer PTC body
// has run past its modelled duration. Synchronously-run bodies set Done
// directly and never carry a ModeledDuration, so they are unaffected.
//
// The observation window is measured against the VIRTUAL clock under the
// deterministic clock / scheduler (where the MTC's `t.timeout` advances
// virtual time, not wall time — so a real-time measure would read ~0 and
// the modelled body would never complete) and against wall time otherwise.
func componentCompleted(ref *runtime.ComponentRef, env runtime.Scope) bool {
	if ref == nil || !ref.Started || ref.ModeledDuration <= 0 {
		return false
	}
	if useVirtualClock(env) {
		if exec := runtime.FindTestcaseExec(env); exec != nil {
			return exec.VirtualClock()-ref.StartedAtVirtual >= ref.ModeledDuration
		}
	}
	if ref.StartedAt.IsZero() {
		return false
	}
	elapsed := time.Since(ref.StartedAt)
	return elapsed >= time.Duration(ref.ModeledDuration*float64(time.Second))
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
// the PTC (deadlock). A forked PTC is woken by its own goDone (no
// deadline); a modelled (skipped, non-forked) PTC parks on its virtual
// completion deadline so the clock advances to it. Returns Undefined once
// the state holds or this participant is stopped.
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
	stop := currentStopChan(exec)
	if !exec.SchedulerActive() {
		// Real clock: wait on the PTC's exit.
		waitPTCDoneRealClock(exec, []*runtime.ComponentRef{ref},
			func() bool { return pred(ref, env) }, stop, env, pred)
		return runtime.NewBool(pred(ref, env))
	}
	for !pred(ref, env) {
		deadline, hasTimer := 0.0, false
		if ref.ModeledDuration > 0 {
			deadline, hasTimer = ref.StartedAtVirtual+ref.ModeledDuration, true
		}
		re, stopped := exec.SchedPark(currentCompID(exec), deadline, hasTimer, stop)
		if stopped || !re {
			break
		}
	}
	return runtime.NewBool(pred(ref, env))
}

// blockUntilComponentsState is the `all component.done` / `any
// component.done` (and `.killed`) analogue of blockUntilComponentState: it
// parks the MTC on the cooperative scheduler until the aggregate predicate
// holds over the given started PTCs, so their forked bodies are granted the
// token and actually run (setting their verdict) rather than the MTC
// racing past a non-blocking snapshot. Each park advances to the soonest
// modelled deadline among the still-unsatisfied PTCs; a forked PTC with no
// modelled duration wakes the park when it finishes.
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
	stop := currentStopChan(exec)
	if !exec.SchedulerActive() {
		waitPTCDoneRealClock(exec, refs, satisfied, stop, env, pred)
		return runtime.NewBool(satisfied())
	}
	for !satisfied() {
		deadline, hasTimer := 0.0, false
		for _, r := range refs {
			if pred(r, env) || r.ModeledDuration <= 0 {
				continue
			}
			if d := r.StartedAtVirtual + r.ModeledDuration; !hasTimer || d < deadline {
				deadline, hasTimer = d, true
			}
		}
		re, stopped := exec.SchedPark(currentCompID(exec), deadline, hasTimer, stop)
		if stopped || !re {
			break
		}
	}
	return runtime.Undefined
}

// compAlive / compRunning / compDone / compKilled are the single source
// of truth for the four component-state predicates. They take env so the
// modelled-completion window can consult the active clock (see
// componentCompleted).
func compAlive(ref *runtime.ComponentRef, env runtime.Scope) bool {
	if ref == nil {
		return false
	}
	if !ref.IsAlive() {
		return false
	}
	// A completed PTC is no longer alive when its behaviour ended and it
	// was not created with the `alive` modifier - or when the modelled
	// body explicitly `kill`ed itself, which removes even an
	// alive-modifier component (ETSI 21.3.4).
	if componentCompleted(ref, env) && (!ref.AliveModifier || ref.ModeledKill) {
		return false
	}
	return true
}

func compRunning(ref *runtime.ComponentRef, env runtime.Scope) bool {
	return ref != nil && ref.IsAlive() && !ref.IsDone() && !componentCompleted(ref, env)
}

func compDone(ref *runtime.ComponentRef, env runtime.Scope) bool {
	return ref == nil || ref.IsDone() || !ref.IsAlive() || componentCompleted(ref, env)
}

func compKilled(ref *runtime.ComponentRef, env runtime.Scope) bool {
	if ref == nil || !ref.IsAlive() {
		return true
	}
	// A completed body kills the component unless it was created `alive`
	// (then it stays reusable) - except when the body itself ran a
	// `kill`, which terminates even an alive-modifier component.
	return componentCompleted(ref, env) && (!ref.AliveModifier || ref.ModeledKill)
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
//   - a modelled PTC (not forked): the real time left until its modelled
//     completion, which the virtual clock would advance to;
//   - a PTC that has exited but is not in the state — an `alive` one whose
//     behaviour ended is done but not killed: nothing announces a later
//     kill, so a slow poll rather than a spin on the closed channel.
//
// A ref that contributes nothing (never forked, nothing modelled) cannot
// change, so it does not stop the wait on the others; only when no ref
// contributes anything does the wait give up and leave the caller the
// non-blocking answer rather than hang. The waiter's own stop signal and
// the testcase being stopped abort it.
func waitPTCDoneRealClock(exec *runtime.TestcaseExec, refs []*runtime.ComponentRef,
	done func() bool, stop <-chan struct{}, env runtime.Scope,
	pred func(*runtime.ComponentRef, runtime.Scope) bool) {

	for !done() {
		cases := []reflect.SelectCase{
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(stop)},
			{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(exec.StopChan())},
		}
		const aborts = 2
		var soonest time.Duration
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
			if d := modelledRemaining(r); d > 0 && (soonest == 0 || d < soonest) {
				soonest = d
			}
		}
		var t *time.Timer
		if soonest > 0 {
			t = time.NewTimer(soonest)
			cases = append(cases, reflect.SelectCase{Dir: reflect.SelectRecv, Chan: reflect.ValueOf(t.C)})
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

// modelledRemaining reports how much real time is left before a PTC whose
// body was modelled rather than forked is considered complete, or 0 when
// nothing is modelled or the duration has already elapsed.
func modelledRemaining(ref *runtime.ComponentRef) time.Duration {
	if ref == nil || !ref.Started || ref.ModeledDuration <= 0 || ref.StartedAt.IsZero() {
		return 0
	}
	total := time.Duration(ref.ModeledDuration * float64(time.Second))
	if remaining := total - time.Since(ref.StartedAt); remaining > 0 {
		return remaining
	}
	return 0
}
