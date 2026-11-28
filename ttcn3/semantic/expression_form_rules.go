// expression_form_rules.go checks the forms of some expressions and
// operands (ETSI ES 201 873-1 clauses 6.2.15.3, 7.1.3, 15.9, A.1.6.8.3,
// B.1.4.2). IfPresent applies to optional fields, not to mandatory ones:
//
//	type record R { integer f1, integer f2 optional }
//	template R mw_rec := { f1 := 1 ifpresent, f2 := omit }; // error
//
// unmap removes a key from a map variable or template variable, not from
// a constant. The
// operands of match are of compatible types. Enumerated values of two
// types are compared when the types merge to a consistent larger one,
// every identifier keeping its integer and no integer naming two
// identifiers. The header of select is no compound expression, and the
// identifier after => names a type.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkExpressionFormRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// template checks the template body v of type t.
	template := func(t, v syntax.Expr) {
		cl, ok := v.(*syntax.CompositeLiteral)
		if !ok {
			return
		}
		r, ok := m.typeRoot(tree, t, 0)
		if !ok || r.decl == nil || r.kind != "record" && r.kind != "set" {
			return
		}
		for _, el := range cl.List {
			be, ok := el.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				continue
			}
			u, ok := be.Y.(*syntax.UnaryExpr)
			if !ok || u.Op == nil || u.Op.Kind() != syntax.IFPRESENT {
				continue
			}
			for _, f := range r.decl.Fields {
				if f != nil && syntax.Name(f.Name) == syntax.Name(be.X) && f.Optional == nil {
					add(u, "ifpresent-mandatory-field", fmt.Sprintf("%s is a mandatory field of %s: ifpresent applies to optional fields (ETSI B.1.4.2 a)", syntax.Name(be.X), r.name))
				}
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			if x.Type != nil && x.Value != nil {
				template(x.Type, x.Value)
			}
		case *syntax.ValueDecl:
			if x.TemplateRestriction != nil && x.Type != nil {
				for _, d := range x.Decls {
					if d != nil && d.Value != nil && len(d.ArrayDef) == 0 {
						template(x.Type, d.Value)
					}
				}
			}
		case *syntax.SelectStmt:
			if _, ok := unparen(x.Tag).(*syntax.CompositeLiteral); ok && x.Union == nil {
				add(x.Tag, "select-compound-header", "the header of select is a single expression, not a compound one (ETSI A.1.6.8.3)")
			}
		case *syntax.BinaryExpr:
			if x.Op == nil {
				return true
			}
			switch x.Op.Kind() {
			case syntax.DECODE:
				id, ok := x.Y.(*syntax.Ident)
				if !ok {
					return true
				}
				for _, d := range tree.LookupWithDB(id, m.db) {
					switch d.Node.(type) {
					case *syntax.ValueDecl, *syntax.Declarator, *syntax.FormalPar, *syntax.TemplateDecl, *syntax.FuncDecl:
						add(id, "decode-target-not-type", fmt.Sprintf("%s names no type: the identifier after => is a type (ETSI A.1.6.8.3)", id))
					}
				}
			case syntax.EQ, syntax.NE:
				l, lok := m.exprRoot(x.X)
				r, rok := m.exprRoot(x.Y)
				if !lok || !rok || l.kind != "enumerated" || r.kind != "enumerated" || l.name == r.name {
					return true
				}
				le, re := m.operandEnum(x.X), m.operandEnum(x.Y)
				if le == nil || re == nil {
					return true
				}
				if why := enumMergeConflict(le, re); why != "" {
					add(x, "enum-merge-inconsistent", fmt.Sprintf("%s and %s do not merge to a consistent enumerated type: %s (ETSI 7.1.3)", l.name, r.name, why))
				}
			}
		case *syntax.CallExpr:
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || x.Args == nil || len(x.Args.List) != 2 || len(tree.LookupWithDB(id, m.db)) > 0 {
				return true
			}
			switch id.String() {
			case "match":
				v, vok := m.exprRoot(x.Args.List[0])
				t, tok := m.exprRoot(x.Args.List[1])
				// A union with a @default alternative matches the
				// alternative's values (6.2.5.2).
				if vok && tok && v.kind != "union" && t.kind != "union" && v.kind != "anytype" && t.kind != "anytype" &&
					(v.structured() && t.primitive() || v.primitive() && t.structured()) {
					add(x, "match-type-incompatible", fmt.Sprintf("a %s value is matched against a %s template (ETSI 15.9)", v.kind, t.kind))
				}
			case "unmap":
				if ref, ok := x.Args.List[0].(*syntax.Ident); ok && m.isMapConstant(ref) {
					add(ref, "unmap-not-variable", fmt.Sprintf("%s is no variable: unmap removes keys from map variables (ETSI 6.2.15.3 a)", ref))
				}
			}
		}
		return true
	})
	return diags
}

// operandEnum is the enumerated type the variable, constant or parameter
// e is of, or nil.
func (m typeModel) operandEnum(e syntax.Expr) *syntax.EnumTypeDecl {
	id, ok := unparen(e).(*syntax.Ident)
	if !ok {
		return nil
	}
	t, tree := m.declType(id)
	if t == nil {
		return nil
	}
	return m.enumDecl(tree, t, 0)
}

// enumMergeConflict tells why the enumerated types a and b do not merge
// to a consistent larger type, or "" when they do or the integers of
// their values are not known.
func enumMergeConflict(a, b *syntax.EnumTypeDecl) string {
	av, aok := enumIntegers(a)
	bv, bok := enumIntegers(b)
	if !aok || !bok {
		return ""
	}
	byInt := map[int]string{}
	for name, n := range av {
		byInt[n] = name
	}
	for name, n := range bv {
		if m, ok := av[name]; ok && m != n {
			return fmt.Sprintf("%s is %d in one and %d in the other", name, m, n)
		}
		if other, ok := byInt[n]; ok && other != name {
			return fmt.Sprintf("%d is both %s and %s", n, other, name)
		}
	}
	return ""
}

// enumIntegers are the integers of the values of ed (6.2.4): the ones
// given, and for the others the least non-negative integer not yet used,
// in order. ok is false when a given integer is no literal.
func enumIntegers(ed *syntax.EnumTypeDecl) (map[string]int, bool) {
	out := map[string]int{}
	used := map[int]bool{}
	for _, e := range ed.Enums {
		ce, ok := e.(*syntax.CallExpr)
		if !ok {
			continue
		}
		if ce.Args == nil || len(ce.Args.List) != 1 {
			return nil, false
		}
		n, ok := signedIntLiteral(ce.Args.List[0])
		if !ok {
			return nil, false
		}
		out[enumLabelName(e)] = int(n)
		used[int(n)] = true
	}
	next := 0
	for _, e := range ed.Enums {
		if _, ok := e.(*syntax.Ident); !ok {
			continue
		}
		for used[next] {
			next++
		}
		out[enumLabelName(e)] = next
		used[next] = true
	}
	return out, true
}

// isMapConstant reports whether id is a constant, module parameter or
// template definition of a map type, no variable.
func (m typeModel) isMapConstant(id *syntax.Ident) bool {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return false
	}
	var t syntax.Expr
	switch d := defs[0].Node.(type) {
	case *syntax.TemplateDecl:
		t = d.Type
	case *syntax.ValueDecl, *syntax.Declarator:
		vd, ok := d.(*syntax.ValueDecl)
		if !ok {
			vd, ok = defs[0].ParentOf(d).(*syntax.ValueDecl)
		}
		if !ok || vd.KindTok == nil {
			return false
		}
		switch vd.KindTok.Kind() {
		case syntax.CONST, syntax.MODULEPAR:
			t = vd.Type
		default:
			return false
		}
	}
	if t == nil {
		return false
	}
	tdefs := defs[0].Tree.LookupWithDB(t, m.db)
	if len(tdefs) != 1 {
		return false
	}
	_, ok := tdefs[0].Node.(*syntax.MapTypeDecl)
	return ok
}
