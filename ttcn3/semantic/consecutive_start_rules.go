package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
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
//
// Only receivers that resolve to a component reference take part:
// `start` on a timer (23.2) or a port (22.5.2) restarts it and needs
// no synchronisation.
func (a *Analyzer) checkConsecutiveStartRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		startSites := map[string]syntax.Node{}
		isComponent := map[string]bool{}
		for _, s := range fn.Body.Stmts {
			es, ok := s.(*syntax.ExprStmt)
			if !ok || es.Expr == nil {
				continue
			}
			recv, op, call := classifyComponentOp(es.Expr)
			if recv == "" {
				continue
			}
			comp, seen := isComponent[recv]
			if !seen {
				comp = a.isComponentRef(tree, componentOpReceiver(call))
				isComponent[recv] = comp
			}
			if !comp {
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

// componentOpReceiver returns the receiver expression of a node
// returned by classifyComponentOp.
func componentOpReceiver(n syntax.Node) syntax.Expr {
	if ce, ok := n.(*syntax.CallExpr); ok {
		n = ce.Fun
	}
	if sel, ok := n.(*syntax.SelectorExpr); ok {
		return sel.X
	}
	return nil
}

// isComponentRef reports whether e resolves to a value of a
// component type. Without a DB only the tree itself is searched.
func (a *Analyzer) isComponentRef(tree *ttcn3.Tree, e syntax.Expr) bool {
	if tree == nil || e == nil {
		return false
	}
	for _, t := range tree.TypeOf(e, a.db()) {
		if _, ok := t.Node.(*syntax.ComponentTypeDecl); ok {
			return true
		}
	}
	return false
}
