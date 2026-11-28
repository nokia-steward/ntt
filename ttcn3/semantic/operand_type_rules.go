// operand_type_rules.go checks the types of the operands of the equality
// and concatenation operators (ETSI ES 201 873-1 clauses 7.1.2, 7.1.3).
// Equality is defined for the types clause 7.1.3 lists, which behaviour
// types are not; an address of a behaviour type is compared with null
// only:
//
//	type function F_t();
//	var F_t v_a := refers(f_a), v_b := refers(f_a);
//	if (v_a == v_b) {} // error
//
// When both operands of a comparison are compound expressions, they are
// value lists whose elements have one root type. The list operands of a
// concatenation have the same outer type and elements of the same root
// type.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkOperandTypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be.Op == nil {
			return true
		}
		switch be.Op.Kind() {
		case syntax.EQ, syntax.NE:
			for _, side := range [][2]syntax.Expr{{be.X, be.Y}, {be.Y, be.X}} {
				name, address := m.behaviourOperand(side[0])
				if name == "" || isNullLiteral(unparen(side[1])) {
					continue
				}
				if address {
					add(side[0], "address-behaviour-comparison", fmt.Sprintf("%s is an address of the behaviour type %s: it is compared with null only (ETSI 7.1.3)", sourceText(side[0]), name))
				} else {
					add(side[0], "behaviour-type-comparison", fmt.Sprintf("%s is of the behaviour type %s, which has no equality (ETSI 7.1.3)", sourceText(side[0]), name))
				}
				return true
			}
			l, lok := unparen(be.X).(*syntax.CompositeLiteral)
			r, rok := unparen(be.Y).(*syntax.CompositeLiteral)
			if !lok || !rok {
				return true
			}
			if isFieldAssignmentList(l) || isFieldAssignmentList(r) {
				add(be, "compound-comparison-notation", "both operands are compound expressions: they are value lists, not field assignments (ETSI 7.1.3)")
				return true
			}
			var first rootType
			for _, el := range append(append([]syntax.Expr{}, l.List...), r.List...) {
				t, ok := m.exprRoot(el)
				if !ok || !t.primitive() {
					continue
				}
				if first.kind == "" {
					first = t
				} else if !comparable(first, t) {
					add(be, "compound-comparison-root-type", fmt.Sprintf("the elements of compared value lists have one root type, not %s and %s (ETSI 7.1.3)", first.kind, t.kind))
					return true
				}
			}
		case syntax.CONCAT:
			x, xok := be.X.(*syntax.Ident)
			y, yok := be.Y.(*syntax.Ident)
			if !xok || !yok {
				return true
			}
			xt, xtree := m.declType(x)
			yt, ytree := m.declType(y)
			if xt == nil || yt == nil {
				return true
			}
			xr, xok := m.typeRoot(xtree, xt, 0)
			yr, yok := m.typeRoot(ytree, yt, 0)
			if !xok || !yok || !isListKind(xr.kind) || !isListKind(yr.kind) {
				return true
			}
			if xr.kind != yr.kind {
				add(be, "concat-list-root-type", fmt.Sprintf("a %s is concatenated with a %s (ETSI 7.1.2)", xr.kind, yr.kind))
				return true
			}
			xe, xok := m.elemRootOfType(xtree, xt)
			ye, yok := m.elemRootOfType(ytree, yt)
			if xok && yok && xe.primitive() && ye.primitive() && xe.kind != ye.kind {
				add(be, "concat-list-root-type", fmt.Sprintf("lists of %s and of %s elements are concatenated: the elements have one root type (ETSI 7.1.2)", xe.kind, ye.kind))
			}
		}
		return true
	})
	return diags
}

// behaviourOperand is the name of the behaviour type the variable,
// constant or parameter e is of, and whether e is an address of it; ""
// when e is of no behaviour type.
func (m typeModel) behaviourOperand(e syntax.Expr) (string, bool) {
	id, ok := unparen(e).(*syntax.Ident)
	if !ok {
		return "", false
	}
	t, tree := m.declType(id)
	address := false
	for depth := 0; t != nil && tree != nil && depth < 8; depth++ {
		defs := tree.LookupWithDB(t, m.db)
		if len(defs) != 1 {
			return "", false
		}
		var f *syntax.Field
		switch d := defs[0].Node.(type) {
		case *syntax.BehaviourTypeDecl:
			return syntax.Name(d.Name), address
		case *syntax.SubTypeDecl:
			f = d.Field
		case *syntax.Field:
			f = d
		}
		if f == nil || len(f.ArrayDef) > 0 {
			return "", false
		}
		if syntax.Name(f.Name) == "address" {
			address = true
		}
		t, tree = typeSpecExpr(f.Type), defs[0].Tree
	}
	return "", false
}

// isFieldAssignmentList reports whether cl assigns fields by name.
func isFieldAssignmentList(cl *syntax.CompositeLiteral) bool {
	for _, el := range cl.List {
		if b, ok := el.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.ASSIGN {
			if _, ok := b.X.(*syntax.Ident); ok {
				return true
			}
		}
	}
	return false
}

func isListKind(k string) bool { return k == "record of" || k == "set of" }
