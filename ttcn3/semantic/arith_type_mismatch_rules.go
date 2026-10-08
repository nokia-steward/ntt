// arith_type_mismatch_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 7.1.1: arithmetic operators
// require both operands to be of the same numeric type.
//
// We only flag the cleanest shape:
//   - the operator is one of `+`, `-`, `*`, `/`, `mod`,
//     `rem`,
//   - both operands are bare identifiers that resolve to
//     a sibling `var <basic-type> name` declared earlier in
//     the same function body,
//   - the two basic types differ (typically integer / float).
//
// Anything more elaborate (literals, member access,
// expressions of derived types, mixed literal+ident) is
// deliberately ignored to avoid regressing tests that
// exploit promotion-style behaviour.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkArithTypeMismatchRules(mod *syntax.Module) []Diagnostic {
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
		walkArithMismatch(fn.Body, map[string]string{}, &diags)
	}
	return diags
}

func walkArithMismatch(
	body *syntax.BlockStmt,
	scope map[string]string,
	diags *[]Diagnostic,
) {
	if body == nil {
		return
	}
	for _, st := range body.Stmts {
		if bs, ok := st.(*syntax.BlockStmt); ok && bs != nil {
			child := map[string]string{}
			for k, v := range scope {
				child[k] = v
			}
			walkArithMismatch(bs, child, diags)
			continue
		}
		ds, ok := st.(*syntax.DeclStmt)
		if ok && ds != nil {
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if ok && vd != nil && vd.KindTok != nil &&
				vd.KindTok.Kind() == syntax.VAR &&
				vd.TemplateRestriction == nil {
				want := basicTypeNameOf(vd.Type)
				for _, dc := range vd.Decls {
					if dc == nil || dc.Name == nil {
						continue
					}
					if want != "" {
						scope[dc.Name.String()] = want
					}
					if dc.Value != nil {
						inspectArith(dc.Value, scope, diags)
					}
				}
				continue
			}
		}
		inspectArith(stmtToExpr(st), scope, diags)
	}
}

func stmtToExpr(st syntax.Stmt) syntax.Expr {
	es, ok := st.(*syntax.ExprStmt)
	if !ok || es == nil {
		return nil
	}
	return es.Expr
}

func inspectArith(expr syntax.Expr, scope map[string]string, diags *[]Diagnostic) {
	if expr == nil {
		return
	}
	syntax.Inspect(expr, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil {
			return true
		}
		if !isArithOp(be.Op.Kind()) {
			return true
		}
		x, ok := be.X.(*syntax.Ident)
		if !ok || x == nil {
			return true
		}
		y, ok := be.Y.(*syntax.Ident)
		if !ok || y == nil {
			return true
		}
		tx, ok := scope[x.String()]
		if !ok {
			return true
		}
		ty, ok := scope[y.String()]
		if !ok {
			return true
		}
		if tx == ty {
			return true
		}
		*diags = append(*diags, Diagnostic{
			Code:     "arith-operand-type-mismatch",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"arithmetic %s on operands %q (%s) and %q (%s) is not allowed; both must share the same numeric type (ETSI 7.1.1)",
				be.Op.String(), x.String(), tx, y.String(), ty),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
}

func isArithOp(k syntax.Kind) bool {
	switch k {
	case syntax.ADD, syntax.SUB, syntax.MUL, syntax.DIV,
		syntax.MOD, syntax.REM:
		return true
	}
	return false
}
