// chained_assign_rules.go enforces ETSI ES 201 873-1
// clause 19.1: an assignment is a statement, not an
// expression. The TTCN-3 grammar therefore does not allow
// the right-hand side of an assignment to embed another
// assignment - constructs of the shape `v_k := (v_j := v_i)`
// are syntactically illegal even when the surrounding
// parser is permissive.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkChainedAssignRules(mod *syntax.Module) []Diagnostic {
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
		for _, st := range fn.Body.Stmts {
			es, ok := st.(*syntax.ExprStmt)
			if !ok || es == nil {
				continue
			}
			be, ok := es.Expr.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil ||
				be.Op.Kind() != syntax.ASSIGN {
				continue
			}
			pe, ok := be.Y.(*syntax.ParenExpr)
			if !ok || pe == nil || len(pe.List) != 1 {
				continue
			}
			inner, ok := pe.List[0].(*syntax.BinaryExpr)
			if !ok || inner == nil || inner.Op == nil ||
				inner.Op.Kind() != syntax.ASSIGN {
				continue
			}
			if _, ok := inner.X.(*syntax.Ident); !ok {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "chained-assignment",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"assignment %q is a statement, not an expression; it cannot be used as the right-hand side of another assignment (ETSI 19.1)",
					stmtRepr(inner)),
				Node: inner,
				Span: syntax.SpanOf(inner),
			})
		}
	}
	return diags
}

func stmtRepr(be *syntax.BinaryExpr) string {
	if be == nil {
		return ""
	}
	lhs := ""
	if id, ok := be.X.(*syntax.Ident); ok && id != nil {
		lhs = id.String()
	}
	rhs := ""
	if id, ok := be.Y.(*syntax.Ident); ok && id != nil {
		rhs = id.String()
	}
	if lhs != "" && rhs != "" {
		return lhs + " := " + rhs
	}
	return lhs + " := ..."
}
