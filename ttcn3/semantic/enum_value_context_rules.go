// enum_value_context_rules.go checks the enumerated values given to a
// variable, constant, module parameter or template of an enumerated type
// (ETSI ES 201 873-1 clause 6.2.4, edition 4). An enumerated value is
// visible in the context of its own type only, so a value of another
// enumerated type is not one of this type's:
//
//	type enumerated E1 { e_a, e_b }
//	type enumerated E2 { e_c, e_d }
//	var E2 v_e := e_a; // error
//
// A value declared with an associated list or range of integers is used
// with one of them in parentheses after its name, never bare.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkEnumValueContextRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil || mod.Edition() != syntax.V4 {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	// owners are the enumerated types declaring each label.
	owners := map[string][]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if ed, ok := n.(*syntax.EnumTypeDecl); ok {
			for _, e := range ed.Enums {
				owners[enumLabelName(e)] = append(owners[enumLabelName(e)], syntax.Name(ed.Name))
			}
			return false
		}
		return true
	})
	var diags []Diagnostic
	check := func(tree *ttcn3.Tree, typ syntax.Expr, v syntax.Expr) {
		ed := m.enumDecl(tree, typ, 0)
		id, ok := v.(*syntax.Ident)
		if ed == nil || !ok || id.Tok == nil || id.Tok.Kind() != syntax.IDENT || id.Tok2 != nil {
			return
		}
		name := id.String()
		for _, e := range ed.Enums {
			if enumLabelName(e) != name {
				continue
			}
			if ce, ok := e.(*syntax.CallExpr); ok && enumHasIntegerList(ce) {
				diags = append(diags, Diagnostic{
					Code:     "enum-value-without-integer",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s of %s has a list of integers: it is used with one of them, %s(n) (ETSI 6.2.4)", name, syntax.Name(ed.Name), name),
					Node:     id,
					Span:     syntax.SpanOf(id),
				})
			}
			return
		}
		if len(owners[name]) == 0 {
			return
		}
		for _, d := range tree.LookupWithDB(id, m.db) {
			switch d.Node.(type) {
			case *syntax.ValueDecl, *syntax.Declarator, *syntax.FormalPar, *syntax.TemplateDecl, *syntax.FuncDecl:
				return
			}
		}
		diags = append(diags, Diagnostic{
			Code:     "enum-value-of-other-type",
			Severity: SeverityError,
			Message:  fmt.Sprintf("%s is a value of %s, not of %s (ETSI 6.2.4)", name, owners[name][0], syntax.Name(ed.Name)),
			Node:     id,
			Span:     syntax.SpanOf(id),
		})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			if x.Type == nil {
				return true
			}
			for _, d := range x.Decls {
				if d != nil && d.Value != nil && len(d.ArrayDef) == 0 {
					check(tree, x.Type, d.Value)
				}
			}
		case *syntax.TemplateDecl:
			if x.Type != nil && x.Value != nil && x.ModifiesTok == nil {
				check(tree, x.Type, x.Value)
			}
		case *syntax.ExprStmt:
			be, ok := x.Expr.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			if id, ok := be.X.(*syntax.Ident); ok {
				if t, tt := m.declType(id); t != nil {
					check(tt, t, be.Y)
				}
			}
		}
		return true
	})
	return diags
}

// enumHasIntegerList reports whether the enumerated value ce declares a
// list of integers or a range, rather than a single integer expression.
func enumHasIntegerList(ce *syntax.CallExpr) bool {
	if ce.Args == nil || len(ce.Args.List) != 1 {
		return ce.Args != nil && len(ce.Args.List) > 1
	}
	b, ok := ce.Args.List[0].(*syntax.BinaryExpr)
	return ok && b.Op != nil && b.Op.Kind() == syntax.RANGE
}

// enumDecl is the enumerated type declaration the type t of tree is, or
// is a subtype of, or nil.
func (m typeModel) enumDecl(tree *ttcn3.Tree, t syntax.Expr, depth int) *syntax.EnumTypeDecl {
	id, ok := t.(*syntax.Ident)
	if !ok || depth > 8 {
		return nil
	}
	defs := tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return nil
	}
	switch d := defs[0].Node.(type) {
	case *syntax.EnumTypeDecl:
		return d
	case *syntax.SubTypeDecl:
		if d.Field != nil && len(d.Field.ArrayDef) == 0 {
			if e := typeSpecExpr(d.Field.Type); e != nil {
				return m.enumDecl(defs[0].Tree, e, depth+1)
			}
		}
	case *syntax.Field:
		if len(d.ArrayDef) == 0 {
			if e := typeSpecExpr(d.Type); e != nil {
				return m.enumDecl(defs[0].Tree, e, depth+1)
			}
		}
	}
	return nil
}
