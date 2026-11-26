// template_data_type_rules.go checks the types templates are of (ETSI ES
// 201 873-1 clause 15 restriction a): data types, component types, and
// structured types made of those only; neither the default type nor a
// port type, nor a type with a field or element of one. That holds for
// template definitions, template variables, formal template parameters,
// the value parameters of templates (5.4.1.1 i) and the template a
// function returns (16.1.0 e).
//
//	type record R { integer f1, default f2 }
//	var template R v_t; // error
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateDataTypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	check := func(t syntax.Expr, what string) {
		if t == nil {
			return
		}
		if kind := m.nonDataPart(tree, t, map[syntax.Node]bool{}); kind != "" {
			diags = append(diags, Diagnostic{
				Code:     "template-non-data-type",
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s is of type %s, which is or contains a %s type: templates are of data and component types (ETSI 15 a)", what, sourceText(t), kind),
				Node:     t,
				Span:     syntax.SpanOf(t),
			})
		}
	}
	params := func(fps *syntax.FormalPars, all bool) {
		if fps == nil {
			return
		}
		for _, p := range fps.List {
			if p != nil && (all || p.TemplateRestriction != nil) {
				check(p.Type, fmt.Sprintf("parameter %s", syntax.Name(p.Name)))
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			check(x.Type, fmt.Sprintf("template %s", syntax.Name(x.Name)))
			params(x.Params, true)
		case *syntax.ValueDecl:
			if x.TemplateRestriction != nil || x.KindTok != nil && x.KindTok.Kind() == syntax.TEMPLATE {
				for _, d := range x.Decls {
					if d != nil {
						check(x.Type, fmt.Sprintf("template %s", syntax.Name(d.Name)))
						break
					}
				}
			}
		case *syntax.FuncDecl:
			params(x.Params, false)
			if x.Return != nil && x.Return.Restriction != nil {
				check(x.Return.Type, fmt.Sprintf("the template %s returns", syntax.Name(x.Name)))
			}
		}
		return true
	})
	return diags
}

// nonDataPart is "default" or "port" when the type t, of tree, is the
// default type or a port type or has a field or element of one, else "".
func (m typeModel) nonDataPart(tree *ttcn3.Tree, t syntax.Node, seen map[syntax.Node]bool) string {
	fields := func(tree *ttcn3.Tree, fs []*syntax.Field) string {
		for _, f := range fs {
			if f != nil {
				if k := m.nonDataPart(tree, f.Type, seen); k != "" {
					return k
				}
			}
		}
		return ""
	}
	switch x := t.(type) {
	case *syntax.RefSpec:
		return m.nonDataPart(tree, x.X, seen)
	case *syntax.StructSpec:
		return fields(tree, x.Fields)
	case *syntax.ListSpec:
		return m.nonDataPart(tree, x.ElemType, seen)
	case *syntax.Ident:
		if x.Tok == nil {
			return ""
		}
		if x.Tok2 == nil && x.String() == "default" {
			return "default"
		}
		defs := tree.LookupWithDB(x, m.db)
		if len(defs) != 1 || seen[defs[0].Node] {
			return ""
		}
		seen[defs[0].Node] = true
		switch d := defs[0].Node.(type) {
		case *syntax.PortTypeDecl:
			return "port"
		case *syntax.StructTypeDecl:
			return fields(defs[0].Tree, d.Fields)
		case *syntax.SubTypeDecl:
			if d.Field != nil {
				return m.nonDataPart(defs[0].Tree, d.Field.Type, seen)
			}
		case *syntax.Field:
			return m.nonDataPart(defs[0].Tree, d.Type, seen)
		}
	}
	return ""
}
