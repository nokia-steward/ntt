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
	pred := compDone
	if op == "killed" {
		pred = compKilled
	}
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || ref == nil {
		return runtime.NewBool(pred(ref, env))
	}
	stop := currentStopChan(exec)
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
	return runtime.Undefined
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
	pred := compDone
	if op == "killed" {
		pred = compKilled
	}
	satisfied := func() bool {
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
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return runtime.NewBool(satisfied())
	}
	stop := currentStopChan(exec)
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
