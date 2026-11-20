// module_def_rules.go checks how module parameters are used and what the
// module definitions part declares (ETSI ES 201 873-1 8.2.0, 8.2.1 a), and
// the right-hand operand of the shift and rotate operators (7.1.6, 7.1.7).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkModuleDefs(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	db := a.db()
	var diags []Diagnostic
	report := func(n syntax.Node, code, format string, args ...any) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: fmt.Sprintf(format, args...), Node: n, Span: syntax.SpanOf(n)})
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		// No variables in the module definitions part (8.2.0).
		if vd, ok := d.Def.(*syntax.ValueDecl); ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.VAR {
			report(vd, "module-variable", "a variable is not declared in the module definitions part (ETSI 8.2.0)")
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.BinaryExpr:
			switch x.Op.Kind() {
			case syntax.ASSIGN:
				// A module parameter is no target of an assignment
				// (8.2.1 a). A field's or a parameter's name in a
				// notation is none.
				if _, ok := tree.ParentOf(x).(*syntax.ExprStmt); ok {
					if root := assignRoot(x.X); root != nil && isModulePar(tree, db, root) {
						report(x, "modulepar-assigned", "module parameter %s is assigned to; module parameters are not changed during test execution (ETSI 8.2.1 a)", root.String())
					}
				}
			case syntax.SHL, syntax.SHR, syntax.ROL, syntax.ROR:
				if negativeLiteral(x.Y) {
					report(x, "negative-shift", "the right-hand operand of %s is a negative integer (ETSI 7.1.6, 7.1.7)", x.Op.Kind())
				}
			}
		}
		return true
	})
	return diags
}

// assignRoot is the variable a target of an assignment refers into: v of
// v, v.f, v[i].
func assignRoot(e syntax.Expr) *syntax.Ident {
	for {
		switch x := e.(type) {
		case *syntax.Ident:
			return x
		case *syntax.SelectorExpr:
			e = x.X
		case *syntax.IndexExpr:
			e = x.X
		default:
			return nil
		}
	}
}

// isModulePar reports whether id refers to a module parameter.
func isModulePar(tree *ttcn3.Tree, db *ttcn3.DB, id *syntax.Ident) bool {
	defs := tree.LookupWithDB(id, db)
	if len(defs) != 1 {
		return false
	}
	vd, ok := defs[0].Node.(*syntax.ValueDecl)
	return ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.MODULEPAR
}

// negativeLiteral reports whether e is a negative integer literal, -n.
func negativeLiteral(e syntax.Expr) bool {
	if p, ok := e.(*syntax.ParenExpr); ok && len(p.List) == 1 {
		e = p.List[0]
	}
	u, ok := e.(*syntax.UnaryExpr)
	if !ok || u.Op.Kind() != syntax.SUB {
		return false
	}
	v, ok := u.X.(*syntax.ValueLiteral)
	return ok && v.Tok != nil && v.Tok.Kind() == syntax.INT && v.Tok.String() != "0"
}
