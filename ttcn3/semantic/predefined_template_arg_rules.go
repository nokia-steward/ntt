// predefined_template_arg_rules.go checks templates and enumerated values
// passed to predefined functions (ETSI ES 201 873-1 annex C). The
// encoding functions encode a specific value, so a template passed to
// them resolves to one, as the argument of a send does:
//
//	template integer m_any := ?;
//	encvalue_unichar(m_any); // error
//
// A template passed to sizeof matches values of one size only, so its
// optional fields are neither AnyValueOrNone nor ifpresent (C.2.2). A
// template of a sequence type passed to substr holds specific values,
// AnyElement and combined templates only, and the subsequence substr
// returns holds no AnyElement (C.4.2). The enumerated value passed to
// enum2int is a typed object, never a bare enumerated label (C.1.30).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPredefinedTemplateArgRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	tmpls := collectTemplateBodies(mod)
	labels := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if ed, ok := n.(*syntax.EnumTypeDecl); ok {
			for _, e := range ed.Enums {
				labels[enumLabelName(e)] = true
			}
			return false
		}
		return true
	})
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || len(tree.LookupWithDB(id, m.db)) > 0 {
			return true
		}
		arg := ce.Args.List[0]
		switch fn := id.String(); fn {
		case "encvalue", "encvalue_unichar", "encvalue_o":
			if !m.isTemplateRef(arg) {
				return true
			}
			if mech := deepMatchingMechanism(arg, tmpls); mech != "" && mech != "omit" && mech != "-" {
				add(arg, "encode-template-not-specific", fmt.Sprintf("%s(%s): the template holds %s, it does not resolve to a specific value (ETSI C.5)", fn, sourceText(arg), mech))
			}
		case "sizeof":
			if !m.isTemplateRef(arg) {
				return true
			}
			cl, ok := m.templateBody(arg).(*syntax.CompositeLiteral)
			if !ok {
				return true
			}
			for _, el := range cl.List {
				be, ok := el.(*syntax.BinaryExpr)
				if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
					continue
				}
				if u, ok := be.Y.(*syntax.UnaryExpr); ok && u.Op != nil && u.Op.Kind() == syntax.IFPRESENT || isBareAnyElementsOrNone(be.Y) {
					add(arg, "sizeof-template-size-varies", fmt.Sprintf("sizeof(%s): field %s is %s, so the values the template matches differ in size (ETSI C.2.2)", sourceText(arg), syntax.Name(be.X), sourceText(be.Y)))
					return true
				}
			}
		case "substr":
			if !m.isTemplateRef(arg) || len(ce.Args.List) != 3 {
				return true
			}
			cl, ok := m.templateBody(arg).(*syntax.CompositeLiteral)
			if !ok {
				return true
			}
			for _, el := range cl.List {
				if p, ok := el.(*syntax.ParenExpr); ok && len(p.List) > 1 {
					add(arg, "substr-forbidden-matching-mechanism", fmt.Sprintf("substr(%s, ...): the template holds the value list %s, which is no specific value, AnyElement or combined template (ETSI C.4.2)", sourceText(arg), sourceText(el)))
					return true
				}
			}
			i, iok := signedIntLiteral(ce.Args.List[1])
			c, cok := signedIntLiteral(ce.Args.List[2])
			if !iok || !cok || i < 0 || c < 0 || i+c > int64(len(cl.List)) {
				return true
			}
			for _, el := range cl.List[i : i+c] {
				if v, ok := el.(*syntax.ValueLiteral); ok && v.Tok != nil && v.Tok.Kind() == syntax.ANY {
					add(arg, "substr-returns-any-element", fmt.Sprintf("substr(%s, %d, %d): the subsequence returned holds AnyElement (ETSI C.4.2)", sourceText(arg), i, c))
					return true
				}
			}
		case "enum2int":
			e, ok := arg.(*syntax.Ident)
			if !ok || e.Tok2 != nil || !labels[e.String()] {
				return true
			}
			for _, d := range tree.LookupWithDB(e, m.db) {
				switch d.Node.(type) {
				case *syntax.ValueDecl, *syntax.Declarator, *syntax.FormalPar, *syntax.TemplateDecl, *syntax.FuncDecl:
					return true
				}
			}
			add(arg, "enum2int-untyped", fmt.Sprintf("enum2int(%s): %s is an enumerated label without a type; pass a typed object (ETSI C.1.30)", e, e))
		}
		return true
	})
	return diags
}
