// component_repeated_call_rules.go enforces ETSI ES 201 873-1
// clause 21.3.10: a non-alive PTC may only be started or
// invoked via `.call()` once. Subsequent `.call()` operations
// on the same variable - or `.call()` after `.start()` - are
// rejected, because once the component terminates it can no
// longer accept new work and the second operation would end
// in a test-case error.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkComponentRepeatedCallRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		nonAlive := collectNonAliveCompVars(fn.Body)
		if len(nonAlive) == 0 {
			continue
		}
		seen := map[string]int{}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			es, ok := n.(*syntax.ExprStmt)
			if !ok || es == nil {
				return true
			}
			ce, ok := es.Expr.(*syntax.CallExpr)
			if !ok || ce == nil {
				se, ok := es.Expr.(*syntax.SelectorExpr)
				if !ok || se == nil {
					return true
				}
				base, opName := unwrapCompOp(se)
				if base == "" || !nonAlive[base] {
					return true
				}
				if opName != "start" {
					return true
				}
				seen[base]++
				if seen[base] > 1 {
					diags = append(diags, Diagnostic{
						Code:     "non-alive-component-reused",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"component %q was created without `alive`; only one `.start`/`.call` is allowed before it terminates (ETSI 21.3.10)",
							base),
						Node: es,
						Span: syntax.SpanOf(es),
					})
				}
				return true
			}
			se, ok := ce.Fun.(*syntax.SelectorExpr)
			if !ok || se == nil {
				return true
			}
			base, opName := unwrapCompOp(se)
			if base == "" || !nonAlive[base] {
				return true
			}
			if opName != "call" && opName != "start" {
				return true
			}
			seen[base]++
			if seen[base] > 1 {
				diags = append(diags, Diagnostic{
					Code:     "non-alive-component-reused",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"component %q was created without `alive`; only one `.start`/`.call` is allowed before it terminates (ETSI 21.3.10)",
						base),
					Node: es,
					Span: syntax.SpanOf(es),
				})
			}
			return true
		})
	}
	return diags
}

func unwrapCompOp(se *syntax.SelectorExpr) (string, string) {
	if se == nil {
		return "", ""
	}
	base, ok := se.X.(*syntax.Ident)
	if !ok || base == nil {
		return "", ""
	}
	sel, ok := se.Sel.(*syntax.Ident)
	if !ok || sel == nil {
		return "", ""
	}
	return base.String(), sel.String()
}

func collectNonAliveCompVars(body *syntax.BlockStmt) map[string]bool {
	out := map[string]bool{}
	if body == nil {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			return true
		}
		if vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil || dc.Value == nil {
				continue
			}
			if isComponentCreateCall(dc.Value) && !isAliveExpr(dc.Value) {
				out[dc.Name.String()] = true
			}
		}
		return true
	})
	return out
}

func isAliveExpr(e syntax.Expr) bool {
	ue, ok := e.(*syntax.UnaryExpr)
	if !ok || ue == nil || ue.Op == nil {
		return false
	}
	return ue.Op.Kind() == syntax.ALIVE
}

func isComponentCreateCall(e syntax.Expr) bool {
	if ue, ok := e.(*syntax.UnaryExpr); ok && ue != nil {
		return isComponentCreateCall(ue.X)
	}
	if ce, ok := e.(*syntax.CallExpr); ok && ce != nil {
		return isComponentCreateCall(ce.Fun)
	}
	se, ok := e.(*syntax.SelectorExpr)
	if !ok || se == nil {
		return false
	}
	sel, ok := se.Sel.(*syntax.Ident)
	if !ok || sel == nil {
		return false
	}
	return sel.String() == "create"
}
