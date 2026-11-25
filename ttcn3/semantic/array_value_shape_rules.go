// array_value_shape_rules.go checks values assigned to multi-dimensional
// arrays (ETSI ES 201 873-1 clause 6.2.7): each dimension that is
// assigned resolves to a set of values enclosed in curly braces, so
//
//	var integer v_a[2][2];
//	v_a := { 1, 2, 3, 4 };
//
// is rejected, while { { 1, 2 }, { 3, 4 } } is not. Only a scalar literal
// where a brace-enclosed list is expected is reported; a reference, which
// may hold an array, and index notation are left alone.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkArrayValueShapeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	check := func(name string, dims int, v syntax.Expr) {
		cl, ok := v.(*syntax.CompositeLiteral)
		if !ok || dims < 2 {
			return
		}
		if e := unbracedDimension(cl, dims); e != nil {
			diags = append(diags, Diagnostic{
				Code:     "array-dimension-not-braced",
				Severity: SeverityError,
				Message: fmt.Sprintf("value of %d-dimensional array %s: %s is not a list of values enclosed in curly braces (ETSI 6.2.7)",
					dims, name, tokenText(e)),
				Node: e,
				Span: syntax.SpanOf(e),
			})
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Declarator:
			if x.Name != nil && x.Value != nil {
				check(x.Name.String(), len(x.ArrayDef), x.Value)
			}
		case *syntax.ExprStmt:
			b, ok := x.Expr.(*syntax.BinaryExpr)
			if !ok || b.Op == nil || b.Op.Kind() != syntax.ASSIGN {
				return true
			}
			id, ok := b.X.(*syntax.Ident)
			if !ok {
				return true
			}
			if _, dc, _, ok := m.valueDeclOf(id); ok {
				check(id.String(), len(dc.ArrayDef), b.Y)
			}
		}
		return true
	})
	return diags
}

// unbracedDimension is the first element of the value cl of an array of
// dims dimensions that is a scalar literal where a list of values is
// expected, or nil.
func unbracedDimension(cl *syntax.CompositeLiteral, dims int) syntax.Expr {
	if dims < 2 {
		return nil
	}
	for _, e := range cl.List {
		switch x := e.(type) {
		case *syntax.CompositeLiteral:
			if bad := unbracedDimension(x, dims-1); bad != nil {
				return bad
			}
		case *syntax.ValueLiteral:
			if x.Tok == nil {
				continue
			}
			switch x.Tok.Kind() {
			case syntax.INT, syntax.FLOAT, syntax.STRING, syntax.BSTRING:
				return x
			}
		}
	}
	return nil
}
