// call_catch_timeout_rules.go enforces ETSI ES 201 873-1
// clause 21.3.10 "The call operation": the optional
// `catch(timeout)` clause attached to a component `call`
// may be present only when the call also supplies a timeout
// value (the second positional argument).
//
// The parser does not fuse the `catch(...)` clause into the
// `call` expression - they are two consecutive statements -
// so the rule scans each BlockStmt for a `catch(timeout)`
// statement whose immediately preceding statement is a
// `<comp>.call(<fn>)` with fewer than two positional
// arguments. The bare `<fn>` form lacks a timeout value and
// therefore cannot be combined with a timeout catch.
package semantic

import (
	"strconv"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkCallCatchTimeoutRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	callees := collectComponentCallCallees(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		blk, ok := n.(*syntax.BlockStmt)
		if !ok || blk == nil {
			return true
		}
		for i := 0; i < len(blk.Stmts); i++ {
			callExpr := componentCallStmtExpr(blk.Stmts[i])
			if callExpr == nil {
				continue
			}
			callee := componentCallTarget(callExpr)
			if callee == "" {
				continue
			}
			fn := callees[callee]
			if fn == nil || fn.Body == nil {
				continue
			}
			next := syntax.Stmt(nil)
			if i+1 < len(blk.Stmts) {
				next = blk.Stmts[i+1]
			}
			if bodyHasBareStop(fn.Body) && !isCatchKindStmt(next, "stop") {
				diags = append(diags, Diagnostic{
					Code:     "component-call-uncaught-stop",
					Severity: SeverityError,
					Message:  "component `call` whose behaviour stops incompletely requires `catch(stop)` (ETSI 21.3.10)",
					Node:     blk.Stmts[i],
					Span:     syntax.SpanOf(blk.Stmts[i]),
				})
			}
			if callTimeout, ok := componentCallTimeout(callExpr); ok {
				if bodyTimeout, ok := bodyBlockingTimeout(fn.Body); ok && bodyTimeout > callTimeout && !isCatchKindStmt(next, "timeout") {
					diags = append(diags, Diagnostic{
						Code:     "component-call-uncaught-timeout",
						Severity: SeverityError,
						Message:  "component `call` whose timeout expires before the behaviour completes requires `catch(timeout)` (ETSI 21.3.10)",
						Node:     blk.Stmts[i],
						Span:     syntax.SpanOf(blk.Stmts[i]),
					})
				}
			}
		}
		for i := 1; i < len(blk.Stmts); i++ {
			if !isCatchTimeoutStmt(blk.Stmts[i]) {
				continue
			}
			callExpr := componentCallStmtExpr(blk.Stmts[i-1])
			if callExpr == nil {
				continue
			}
			argCount := 0
			if callExpr.Args != nil {
				argCount = len(callExpr.Args.List)
			}
			if argCount >= 2 {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "call-catch-timeout-without-value",
				Severity: SeverityError,
				Message:  "`catch(timeout)` on a component `call` requires the call to declare a timeout value (second argument) (ETSI 21.3.10)",
				Node:     blk.Stmts[i],
				Span:     syntax.SpanOf(blk.Stmts[i]),
			})
		}
		return true
	})
	return diags
}

// isCatchTimeoutStmt reports whether the statement is the bare
// `catch(timeout)` form the parser emits as a standalone
// ExprStmt right after the `call` it qualifies.
func isCatchTimeoutStmt(stmt syntax.Stmt) bool {
	return isCatchKindStmt(stmt, "timeout")
}

func isCatchKindStmt(stmt syntax.Stmt, kind string) bool {
	es, ok := stmt.(*syntax.ExprStmt)
	if !ok || es == nil {
		return false
	}
	ce, ok := es.Expr.(*syntax.CallExpr)
	if !ok || ce == nil {
		return false
	}
	id, ok := ce.Fun.(*syntax.Ident)
	if !ok || id == nil || id.String() != "catch" {
		return false
	}
	if ce.Args == nil || len(ce.Args.List) == 0 {
		return false
	}
	arg, ok := ce.Args.List[0].(*syntax.Ident)
	return ok && arg != nil && arg.String() == kind
}

// componentCallStmtExpr returns the `<comp>.call(...)` CallExpr
// when the statement is exactly that ExprStmt shape; otherwise nil.
// We require a SelectorExpr-on-CallExpr to avoid matching the bare
// `call(<sig>)` port operation form.
func componentCallStmtExpr(stmt syntax.Stmt) *syntax.CallExpr {
	es, ok := stmt.(*syntax.ExprStmt)
	if !ok || es == nil {
		return nil
	}
	expr := es.Expr
	if r, ok := expr.(*syntax.RedirectExpr); ok && r != nil {
		expr = r.X
	}
	ce, ok := expr.(*syntax.CallExpr)
	if !ok || ce == nil {
		return nil
	}
	sel, ok := ce.Fun.(*syntax.SelectorExpr)
	if !ok || sel == nil {
		return nil
	}
	op, ok := sel.Sel.(*syntax.Ident)
	if !ok || op == nil || op.String() != "call" {
		return nil
	}
	return ce
}

func collectComponentCallCallees(mod *syntax.Module) map[string]*syntax.FuncDecl {
	out := map[string]*syntax.FuncDecl{}
	if mod == nil {
		return out
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Name == nil {
			continue
		}
		out[fn.Name.String()] = fn
	}
	return out
}

func componentCallTarget(call *syntax.CallExpr) string {
	if call == nil || call.Args == nil || len(call.Args.List) == 0 {
		return ""
	}
	ce, ok := call.Args.List[0].(*syntax.CallExpr)
	if !ok || ce == nil {
		return ""
	}
	id, ok := ce.Fun.(*syntax.Ident)
	if !ok || id == nil {
		return ""
	}
	return id.String()
}

func bodyHasBareStop(body *syntax.BlockStmt) bool {
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		if found || n == nil {
			return false
		}
		es, ok := n.(*syntax.ExprStmt)
		if !ok || es == nil {
			return true
		}
		id, ok := es.Expr.(*syntax.Ident)
		if ok && id != nil && id.String() == "stop" {
			found = true
			return false
		}
		return true
	})
	return found
}

func componentCallTimeout(call *syntax.CallExpr) (float64, bool) {
	if call == nil || call.Args == nil || len(call.Args.List) < 2 {
		return 0, false
	}
	return literalFloatValue(call.Args.List[1])
}

func bodyBlockingTimeout(body *syntax.BlockStmt) (float64, bool) {
	if body == nil {
		return 0, false
	}
	timerDefaults := map[string]float64{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		isTimer := vd.KindTok != nil && vd.KindTok.Kind() == syntax.TIMER
		if id, ok := vd.Type.(*syntax.Ident); ok && id.Tok != nil && id.Tok.Kind() == syntax.TIMER {
			isTimer = true
		}
		if !isTimer {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			if f, ok := literalFloatValue(dec.Value); ok {
				timerDefaults[dec.Name.String()] = f
			}
		}
		return true
	})
	var max float64
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok || op == nil || op.String() != "timeout" {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base == nil {
			return true
		}
		if f, ok := timerDefaults[base.String()]; ok {
			if !found || f > max {
				max = f
				found = true
			}
		}
		return true
	})
	return max, found
}

func literalFloatValue(expr syntax.Expr) (float64, bool) {
	lit, ok := expr.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil || lit.Tok.Kind() != syntax.FLOAT {
		return 0, false
	}
	f, err := strconv.ParseFloat(lit.Tok.String(), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
