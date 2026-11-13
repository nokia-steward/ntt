package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkNonAliveRestartRules enforces ETSI 21.3.{2,3,4}: a test
// component that was created WITHOUT the `alive` modifier is a
// one-shot resource. Once it completes - via `stop`, `kill`,
// `done`, `killed`, or just by falling off the end of its body -
// it is destroyed; any subsequent `start` on the same reference
// is a static test-case error.
//
// The check works on a per-function basis and only follows the
// top-level statement list (we already make this concession in
// consecutive_start_rules.go). For every component variable we
// have to know two things:
//
//  1. Was it created without `alive`? We see this when the
//     initialiser of a `var C x := ...` or a bare assignment
//     `x := ...` is a `<Type>.create` CallExpr NOT wrapped in
//     `UnaryExpr{Op: alive}`.
//  2. After we see `x.start`, has any sync op marked it as
//     consumed (`x.stop`, `x.kill`, `x.done`, `x.killed`)?
//
// Once both conditions are met, any further `x.start(...)` is
// flagged. Aliased components (`var C x := C.create alive`) are
// excluded - they may legitimately be restarted.
func (a *Analyzer) checkNonAliveRestartRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		// per-function: name -> false (non-alive) /
		// true (alive) / missing (unknown).
		aliveness := map[string]bool{}
		// per-function flags. `consumed` means a non-alive
		// component has hit a sync op; `killed` means even
		// an alive component has been explicitly destroyed.
		consumed := map[string]bool{}
		killed := map[string]bool{}
		for _, s := range fn.Body.Stmts {
			recordAliveness(s, aliveness)
			es, ok := s.(*syntax.ExprStmt)
			if !ok || es.Expr == nil {
				continue
			}
			recv, op, call := classifyComponentOp(es.Expr)
			if recv == "" {
				continue
			}
			alive, known := aliveness[recv]
			switch op {
			case "start":
				switch {
				case killed[recv]:
					diags = append(diags, Diagnostic{
						Code:     "killed-restart",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"`%s` was destroyed by `.kill`; restarting a killed component is forbidden (ETSI 21.3.4)",
							recv),
						Node: call,
						Span: syntax.SpanOf(call),
					})
				case known && !alive && consumed[recv]:
					diags = append(diags, Diagnostic{
						Code:     "nonalive-restart",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"`%s` was created without `alive`; restarting after `.stop`/`.kill`/`.done` is forbidden (ETSI 21.3.{2,3,4})",
							recv),
						Node: call,
						Span: syntax.SpanOf(call),
					})
				}
				consumed[recv] = false
			case "stop", "done":
				if known && !alive {
					consumed[recv] = true
				}
			case "kill", "killed":
				// `kill` destroys even alive PTCs.
				killed[recv] = true
				if known && !alive {
					consumed[recv] = true
				}
			}
		}
		return true
	})
	return diags
}

// recordAliveness updates aliveness for any component creation
// reachable from a single top-level statement. Both `var C x :=
// C.create [alive]` (DeclStmt → ValueDecl) and `x := C.create
// [alive]` (ExprStmt → BinaryExpr `:=`) are recognised. Anything
// else leaves aliveness untouched.
func recordAliveness(stmt syntax.Stmt, aliveness map[string]bool) {
	switch v := stmt.(type) {
	case *syntax.DeclStmt:
		if v == nil || v.Decl == nil {
			return
		}
		vd, ok := v.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil || d.Value == nil {
				continue
			}
			if alive, isCreate := classifyCreateExpr(d.Value); isCreate {
				aliveness[d.Name.String()] = alive
			}
		}
	case *syntax.ExprStmt:
		if v == nil || v.Expr == nil {
			return
		}
		bin, ok := v.Expr.(*syntax.BinaryExpr)
		if !ok || bin == nil || bin.Op == nil {
			return
		}
		if bin.Op.String() != ":=" {
			return
		}
		lhs, ok := bin.X.(*syntax.Ident)
		if !ok || lhs == nil {
			return
		}
		if alive, isCreate := classifyCreateExpr(bin.Y); isCreate {
			aliveness[lhs.String()] = alive
		}
	}
}

// classifyCreateExpr inspects e and reports (alive, isCreate).
// isCreate is true when e is structurally `Type.create(...)` or
// `Type.create(...) alive`. alive is true only in the latter
// case.
func classifyCreateExpr(e syntax.Expr) (alive bool, isCreate bool) {
	if u, ok := e.(*syntax.UnaryExpr); ok && u != nil && u.Op != nil && u.Op.String() == "alive" {
		_, ok := isCreateCall(u.X)
		return ok, ok
	}
	_, ok := isCreateCall(e)
	return false, ok
}

// isCreateCall reports whether e is the `<X>.create` /
// `<X>.create(...)` call shape. Returns the call expression so
// callers can dig deeper if they want; we just use the boolean.
func isCreateCall(e syntax.Expr) (*syntax.CallExpr, bool) {
	if ce, ok := e.(*syntax.CallExpr); ok && ce != nil && ce.Fun != nil {
		if sel, ok := ce.Fun.(*syntax.SelectorExpr); ok && sel != nil {
			if id, ok := sel.Sel.(*syntax.Ident); ok && id != nil && id.String() == "create" {
				return ce, true
			}
		}
	}
	if sel, ok := e.(*syntax.SelectorExpr); ok && sel != nil {
		if id, ok := sel.Sel.(*syntax.Ident); ok && id != nil && id.String() == "create" {
			return nil, true
		}
	}
	return nil, false
}
