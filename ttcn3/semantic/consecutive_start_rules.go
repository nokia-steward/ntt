package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkConsecutiveStartRules enforces ETSI 21.3.2: a second
// `v.start(...)` on the same component reference must be preceded
// by a synchronisation operation that makes the previous start
// observable as completed - `v.done`, `v.stop`, `v.kill`,
// `v.killed`, `v.running`, `v.alive`, or a `done`/`killed` /
// `running` / `alive` predicate test. Without any of those, the
// second start is a test-case error regardless of whether the
// component was created with the `alive` modifier.
//
// We scan each function body's top-level statement list and track
// a per-identifier "needs sync before next start" flag. The flag
// is set on each `v.start(...)` and cleared by any synchronising
// operation on the same identifier in source order. A start that
// fires while the flag is set is flagged.
//
// The check is intentionally narrow: it only inspects the top
// level of a function / testcase body. Nested if/while shapes are
// skipped because the scheduler can interleave them in ways that
// make a purely syntactic rule unsound.
func (a *Analyzer) checkConsecutiveStartRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	// Timer names are excluded from the component start/sync
	// pairing rule: `T.start` on a timer is idempotent (it
	// restarts the wall clock from zero) and explicitly NOT
	// covered by ETSI 21.3.2. This is module-wide because the
	// receiver could be a component-body timer reached via a
	// `runs on` clause.
	timers := collectModuleTimerIdents(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		startSites := map[string]syntax.Node{}
		for _, s := range fn.Body.Stmts {
			es, ok := s.(*syntax.ExprStmt)
			if !ok || es.Expr == nil {
				continue
			}
			recv, op, call := classifyComponentOp(es.Expr)
			if recv == "" {
				continue
			}
			if timers[recv] {
				// Timer ops follow ETSI 23, not 21.3.2.
				continue
			}
			switch op {
			case "start":
				if prev, ok := startSites[recv]; ok && prev != nil {
					diags = append(diags, Diagnostic{
						Code:     "start-without-sync",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"`%s.start(...)` called twice without an intervening `.done`/`.stop`/`.kill` (ETSI 21.3.2)",
							recv),
						Node: call,
						Span: syntax.SpanOf(call),
					})
				}
				startSites[recv] = call
			case "done", "stop", "kill", "killed", "running", "alive":
				delete(startSites, recv)
			}
		}
		return true
	})
	return diags
}

// classifyComponentOp extracts (receiver-ident, op-name, callExpr)
// from an expression statement that's either `v.op(...)` (a call)
// or `v.op` (a bare selector). Returns "" for the receiver when
// the expression isn't a recognisable component-method call.
//
// The callExpr return value is the node we attach the diagnostic
// to; for bare-selector statements it's the synthetic enclosing
// CallExpr-like, so the caller may pass it through SpanOf without
// special-casing.
func classifyComponentOp(e syntax.Expr) (string, string, syntax.Node) {
	if ce, ok := e.(*syntax.CallExpr); ok && ce.Fun != nil {
		if sel, ok := ce.Fun.(*syntax.SelectorExpr); ok {
			if recv, op := identName(sel.X), identName(sel.Sel); recv != "" && op != "" {
				return recv, op, ce
			}
		}
	}
	if sel, ok := e.(*syntax.SelectorExpr); ok {
		if recv, op := identName(sel.X), identName(sel.Sel); recv != "" && op != "" {
			return recv, op, sel
		}
	}
	return "", "", nil
}
