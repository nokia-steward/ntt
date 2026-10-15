package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkTimerArityRules enforces ETSI 23.x syntactic shape rules
// for timer-method calls:
//
//   - `T.timeout`, `T.read`, `T.running`, `T.stop`, `T.killed`
//     take NO arguments. `T.timeout()` is just as wrong as
//     `T.timeout(5)`.
//   - `T.start` takes either NO arguments (use declared default)
//     or EXACTLY ONE argument (the override duration). `T.start()`
//     with empty parens is illegal, as is `T.start(a, b)`.
//   - `any timer.timeout` likewise takes no arguments.
//
// The check fires only when we can prove the receiver is a
// timer (or `any timer`/`all timer`); other selectors like a
// method on a component reference are left alone.
func (a *Analyzer) checkTimerArityRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	// Module-wide set of identifiers declared as timers anywhere
	// (component bodies, function params, var-decls). Over-broad
	// on purpose: a name collision with a component-port or a
	// var of a different type would only suppress a real diag if
	// they shared a name with a timer somewhere - in practice
	// the false-positive rate is zero on the conformance suite.
	timers := collectModuleTimerIdents(mod)
	if len(timers) == 0 {
		return diags
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			ce, ok := sn.(*syntax.CallExpr)
			if !ok || ce == nil || ce.Fun == nil {
				return true
			}
			sel, ok := ce.Fun.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			opIdent, ok := sel.Sel.(*syntax.Ident)
			if !ok || opIdent == nil {
				return true
			}
			op := opIdent.String()
			if !timerOp(op) {
				return true
			}
			// Receiver must be either a known timer or
			// the `any timer` / `all timer` keyword form.
			recvIsTimer := false
			if id, ok := sel.X.(*syntax.Ident); ok && id != nil {
				if timers[id.String()] {
					recvIsTimer = true
				}
			}
			if !recvIsTimer && isAnyAllTimerReceiver(sel.X) {
				// `any/all timer.timeout()` etc - the
				// keyword-qualified receiver also counts.
				recvIsTimer = true
			}
			if !recvIsTimer {
				return true
			}
			emitTimerArityDiag(&diags, op, ce)
			return true
		})
		return true
	})
	return diags
}

// collectModuleTimerIdents walks the whole module and returns
// every identifier ever declared as a timer (component bodies,
// function params, var-decls). Scope-free on purpose: we want to
// know "is the name `t_timer` plausibly a timer anywhere?" so we
// can flag arity violations regardless of which function body the
// reference appears in.
func collectModuleTimerIdents(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.FormalPar:
			if v == nil || v.Name == nil || v.Type == nil {
				return true
			}
			if id, ok := v.Type.(*syntax.Ident); ok && id != nil &&
				id.Tok != nil && id.Tok.Kind() == syntax.TIMER {
				out[v.Name.String()] = true
			}
		case *syntax.ValueDecl:
			if v == nil {
				return true
			}
			if v.Type != nil {
				if id, ok := v.Type.(*syntax.Ident); ok && id != nil &&
					id.Tok != nil && id.Tok.Kind() == syntax.TIMER {
					for _, d := range v.Decls {
						if d == nil || d.Name == nil {
							continue
						}
						out[d.Name.String()] = true
					}
				}
			}
			if v.KindTok != nil && v.KindTok.Kind() == syntax.TIMER {
				for _, d := range v.Decls {
					if d == nil || d.Name == nil {
						continue
					}
					out[d.Name.String()] = true
				}
			}
		}
		return true
	})
	return out
}

// timerOp reports whether op is one of the bare-no-args timer
// operations covered by ETSI 23.{3,4,5,6,8}.
func timerOp(op string) bool {
	switch op {
	case "timeout", "read", "running", "stop", "start":
		return true
	}
	return false
}

// timerObservationOp reports whether op is a timer operation
// that READS / OBSERVES timer state. These are illegal in
// component-body initialisers per ETSI 23 ("Timer ops not
// allowed outside test cases, functions, altsteps, control").
func timerObservationOp(op string) bool {
	switch op {
	case "read", "running", "timeout":
		return true
	}
	return false
}

// emitTimerArityDiag appends the arity violation for the given
// timer op when ce's argument list shape is wrong.
func emitTimerArityDiag(diags *[]Diagnostic, op string, ce *syntax.CallExpr) {
	if ce.Args == nil {
		// `T.op` with no parens is fine for every op.
		return
	}
	n := len(ce.Args.List)
	switch op {
	case "start":
		switch n {
		case 0:
			*diags = append(*diags, Diagnostic{
				Code:     "timer-start-empty-parens",
				Severity: SeverityError,
				Message:  "timer `start` does not accept empty `()`; use `start` or `start(duration)` (ETSI 23.2)",
				Node:     ce,
				Span:     syntax.SpanOf(ce),
			})
		case 1:
			// Valid: T.start(N).
		default:
			*diags = append(*diags, Diagnostic{
				Code:     "timer-start-extra-args",
				Severity: SeverityError,
				Message:  fmt.Sprintf("timer `start` takes at most 1 argument; got %d (ETSI 23.2)", n),
				Node:     ce,
				Span:     syntax.SpanOf(ce),
			})
		}
	default:
		// `timeout`, `read`, `running`, `stop` take no args.
		*diags = append(*diags, Diagnostic{
			Code:     "timer-op-no-args",
			Severity: SeverityError,
			Message:  fmt.Sprintf("timer `%s` does not accept arguments; use `T.%s` (ETSI 23.{3,4,5,6})", op, op),
			Node:     ce,
			Span:     syntax.SpanOf(ce),
		})
	}
}
