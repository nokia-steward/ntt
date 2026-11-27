// template_optional_field_rules.go checks that a template definition in
// assignment notation initialises the optional fields of its records and
// sets (ETSI ES 201 873-1 clauses 15.1, 15.2). An optional field left
// out is initialised implicitly only when an optional "implicit omit"
// attribute applies to the template:
//
//	type record MyRecord { integer field1 optional, charstring field2 }
//	template MyRecord m_rec := { field2 := "a" }; // error
//
// The fields of signature parameters are checked alike. A modified or
// @abstract template may leave fields out, and a module with any
// optional attribute is let be: which one applies is not looked at.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateOptionalFieldRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	optional := false
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if w, ok := n.(*syntax.WithStmt); ok && w.KindTok != nil && w.KindTok.Kind() == syntax.OPTIONAL {
			optional = true
		}
		return !optional
	})
	if optional {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	// fields checks the value v given for a record or set of root type r.
	var fields func(name string, r rootType, v syntax.Expr, depth int)
	fields = func(name string, r rootType, v syntax.Expr, depth int) {
		cl, ok := v.(*syntax.CompositeLiteral)
		if !ok || depth > 8 || r.decl == nil || r.kind != "record" && r.kind != "set" {
			return
		}
		given := map[string]syntax.Expr{}
		for _, el := range cl.List {
			be, ok := el.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return
			}
			given[syntax.Name(be.X)] = be.Y
		}
		for _, f := range r.decl.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			fv, ok := given[f.Name.String()]
			if !ok {
				if f.Optional != nil {
					diags = append(diags, Diagnostic{
						Code:     "template-optional-field-uninitialised",
						Severity: SeverityError,
						Message:  fmt.Sprintf("template %s does not initialise the optional field %s of %s, and no optional \"implicit omit\" attribute applies (ETSI 15.1)", name, f.Name, r.name),
						Node:     cl,
						Span:     syntax.SpanOf(cl),
					})
				}
				continue
			}
			if len(f.ArrayDef) > 0 {
				continue
			}
			if e := typeSpecExpr(f.Type); e != nil {
				if fr, ok := m.typeRoot(r.tree, e, 0); ok {
					fields(name, fr, fv, depth+1)
				}
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td.ModifiesTok != nil || td.Modif != nil || td.Type == nil || td.Value == nil {
			return true
		}
		name := syntax.Name(td.Name)
		defs := tree.LookupWithDB(td.Type, m.db)
		if len(defs) == 1 {
			if sig, ok := defs[0].Node.(*syntax.SignatureDecl); ok {
				cl, ok := td.Value.(*syntax.CompositeLiteral)
				if !ok || sig.Params == nil {
					return true
				}
				for _, el := range cl.List {
					be, ok := el.(*syntax.BinaryExpr)
					if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
						continue
					}
					for _, p := range sig.Params.List {
						if p != nil && syntax.Name(p.Name) == syntax.Name(be.X) && len(p.ArrayDef) == 0 {
							if r, ok := m.typeRoot(defs[0].Tree, p.Type, 0); ok {
								fields(name, r, be.Y, 0)
							}
						}
					}
				}
				return true
			}
		}
		if r, ok := m.typeRoot(tree, td.Type, 0); ok {
			fields(name, r, td.Value, 0)
		}
		return true
	})
	return diags
}
