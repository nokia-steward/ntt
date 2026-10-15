// altstep_invoke_rules.go enforces a narrow flavour of ETSI
// ES 201 873-1 clause 16.2.1: invoking an altstep from an
// `alt`-branch is only allowed when the altstep's `runs on`
// component is compatible with the caller's `runs on`
// component (or they are related through the component
// extension graph).
//
// Pattern matched:
//
//	alt {
//	    [] altstepName(...);
//	    ...
//	}
//
// We use the same component-compatibility predicate as the
// `activate` rule (component_compatible / extends graph).
// When the altstep cannot be resolved by name (e.g.
// imported from another module) we silently skip.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAltstepInvokeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	funcs := collectFunctionSignatures(mod)
	if len(funcs) == 0 {
		return nil
	}
	extends := collectComponentExtensionGraph(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		callerComp := runsOnComponent(fn)
		if callerComp == "" {
			continue
		}
		walkAltBranches(fn.Body, callerComp, funcs, extends, &diags)
	}
	return diags
}

func walkAltBranches(
	body *syntax.BlockStmt,
	callerComp string,
	funcs map[string]*syntax.FuncDecl,
	extends map[string]map[string]bool,
	diags *[]Diagnostic,
) {
	if body == nil {
		return
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		as, ok := n.(*syntax.AltStmt)
		if !ok || as == nil || as.Body == nil {
			return true
		}
		for _, st := range as.Body.Stmts {
			cc, ok := st.(*syntax.CommClause)
			if !ok || cc == nil {
				continue
			}
			ce := unwrapAltstepInvocation(cc.Comm)
			if ce == nil {
				continue
			}
			id, ok := ce.Fun.(*syntax.Ident)
			if !ok || id == nil {
				continue
			}
			decl, found := funcs[id.String()]
			if !found || decl == nil || decl.KindTok == nil {
				continue
			}
			if decl.KindTok.Kind() != syntax.ALTSTEP {
				continue
			}
			targetComp := runsOnComponent(decl)
			if targetComp == "" {
				continue
			}
			if componentCompatible(callerComp, targetComp, extends) {
				continue
			}
			*diags = append(*diags, Diagnostic{
				Code:     "altstep-invoke-runs-on-mismatch",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"altstep %q runs on %q but the caller runs on %q; the component types are unrelated (ETSI 16.2.1)",
					id.String(), targetComp, callerComp),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
		}
		return true
	})
}

// unwrapAltstepInvocation returns the altstep CallExpr inside
// a CommClause's `Comm` slot when it has the shape
// `altstepName(args...)` (either as a bare ExprStmt or
// wrapped in an ExprStmt). Returns nil for any other shape.
func unwrapAltstepInvocation(stmt syntax.Stmt) *syntax.CallExpr {
	if stmt == nil {
		return nil
	}
	es, ok := stmt.(*syntax.ExprStmt)
	if !ok || es == nil || es.Expr == nil {
		return nil
	}
	ce, ok := es.Expr.(*syntax.CallExpr)
	if !ok || ce == nil {
		return nil
	}
	if _, ok := ce.Fun.(*syntax.Ident); !ok {
		return nil
	}
	return ce
}
