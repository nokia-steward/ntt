// type_rules.go enforces type compatibility rules of ETSI ES 201 873-1 on
// the root-type model (type_model.go), for the basic and string types
// only so far:
//
//   - a value assigned or given as initial value is of the target's root
//     type, a charstring into a universal charstring allowed (6.3.1, 19.1);
//   - the operands of a relational operator are of compatible root types
//     (7.1.3);
//   - the operands of and, or, xor and not are boolean (7.1.4);
//   - an actual parameter given to an in formal parameter of a function,
//     altstep or testcase is of its root type (5.4.2 d);
//   - the templates of the branches of a select case are of the header's
//     type (19.3.1), the header of a select union is a union (19.3.2).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	funcs := collectFuncDeclsByName(mod)
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// primitive pair of known root types, or ok false.
	both := func(x, y syntax.Expr) (rootType, rootType, bool) {
		l, lok := m.exprRoot(x)
		r, rok := m.exprRoot(y)
		return l, r, lok && rok && l.primitive() && r.primitive()
	}
	mod.Inspect(func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			if x.KindTok == nil || x.Type == nil || x.TemplateRestriction != nil {
				return true
			}
			switch x.KindTok.Kind() {
			case syntax.VAR, syntax.CONST, syntax.MODULEPAR:
			default:
				return true
			}
			t, ok := m.typeRoot(tree, x.Type, 0)
			if !ok || !t.primitive() {
				return true
			}
			for _, d := range x.Decls {
				if d == nil || d.Value == nil || len(d.ArrayDef) > 0 {
					continue
				}
				if v, ok := m.exprRoot(d.Value); ok && v.primitive() && !assignable(t, v) {
					add(d, "type-incompatible-value", fmt.Sprintf("a %s value is no initial value of the %s %s (ETSI 6.3.1)", v.kind, t.kind, syntax.Name(d.Name)))
				}
			}
		case *syntax.BinaryExpr:
			if x.Op == nil {
				return true
			}
			switch x.Op.Kind() {
			case syntax.ASSIGN:
				switch target := x.X.(type) {
				case *syntax.Ident, *syntax.SelectorExpr:
					if isAssignStatement(tree, x) {
						if t, v, ok := both(target, x.Y); ok && !assignable(t, v) {
							add(x, "type-incompatible-assignment", fmt.Sprintf("a %s value is assigned to the %s %s (ETSI 6.3.1, 19.1)", v.kind, t.kind, syntax.Name(target)))
						}
					}
				}
			case syntax.LT, syntax.GT, syntax.LE, syntax.GE, syntax.EQ, syntax.NE:
				if l, r, ok := both(x.X, x.Y); ok && !comparable(l, r) {
					add(x, "type-incompatible-comparison", fmt.Sprintf("a %s value is compared with a %s value (ETSI 7.1.3)", l.kind, r.kind))
				}
			case syntax.AND, syntax.OR, syntax.XOR:
				for _, side := range []syntax.Expr{x.X, x.Y} {
					if r, ok := m.exprRoot(side); ok && r.primitive() && r.kind != "boolean" {
						add(side, "type-boolean-operand", fmt.Sprintf("an operand of %s is a %s value, not boolean (ETSI 7.1.4)", x.Op.String(), r.kind))
					}
				}
			}
		case *syntax.SelectStmt:
			if x.Tag == nil {
				return true
			}
			h, ok := m.exprRoot(x.Tag)
			if !ok {
				return true
			}
			// The header of a select union is of a union type or
			// anytype (19.3.2).
			if x.Union != nil {
				if h.primitive() || h.kind == "enumerated" || h.kind == "record" || h.kind == "set" {
					add(x.Tag, "select-union-header", fmt.Sprintf("the header of select union is a %s value, not a union (ETSI 19.3.2)", h.kind))
				}
				return true
			}
			if !h.primitive() {
				return true
			}
			// The templates of the branches are of the header's type
			// (19.3.1).
			for _, c := range x.Body {
				if c == nil || c.Case == nil {
					continue
				}
				for _, e := range c.Case.List {
					if v, ok := m.exprRoot(e); ok && v.primitive() && !comparable(h, v) {
						add(e, "select-case-type", fmt.Sprintf("a %s branch of a select on a %s value (ETSI 19.3.1)", v.kind, h.kind))
					}
				}
			}
		case *syntax.UnaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.NOT {
				if r, ok := m.exprRoot(x.X); ok && r.primitive() && r.kind != "boolean" {
					add(x, "type-boolean-operand", fmt.Sprintf("the operand of not is a %s value, not boolean (ETSI 7.1.4)", r.kind))
				}
			}
		case *syntax.CallExpr:
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || x.Args == nil {
				return true
			}
			fd := funcs[id.String()]
			if fd == nil || fd.Params == nil {
				return true
			}
			for i, arg := range x.Args.List {
				if _, named := namedArgName(arg); named || i >= len(fd.Params.List) {
					break
				}
				p := fd.Params.List[i]
				if p == nil || p.Type == nil || len(p.ArrayDef) > 0 || p.TemplateRestriction != nil {
					continue
				}
				if p.Direction != nil && p.Direction.Kind() != syntax.IN {
					continue
				}
				t, tok := m.typeRoot(tree, p.Type, 0)
				v, vok := m.exprRoot(arg)
				if tok && vok && t.primitive() && v.primitive() && !assignable(t, v) {
					add(arg, "type-incompatible-actual", fmt.Sprintf("a %s value is given to the %s formal parameter %s (ETSI 5.4.2 d)", v.kind, t.kind, syntax.Name(p.Name)))
				}
			}
		}
		return true
	})
	return diags
}

// isAssignStatement reports whether the assignment b is a statement or a
// for loop's, not a field or parameter assignment of a notation.
func isAssignStatement(tree *ttcn3.Tree, b *syntax.BinaryExpr) bool {
	n := &ttcn3.Node{Node: b, Tree: tree}
	switch n.ParentOf(b).(type) {
	case *syntax.ExprStmt, *syntax.ForStmt:
		return true
	}
	return false
}
