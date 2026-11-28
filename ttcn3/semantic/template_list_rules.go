// template_list_rules.go checks template lists and complemented template
// lists (ETSI ES 201 873-1 B.1.2.1, B.1.2.2) in template definitions,
// against the type the list is used for: the template's, or a field's of
// it.
//
//   - Each value in the list is of that type (B.1.2.1 d, B.1.2.2 d).
//   - The template of an `all from` clause is of a record of or set of
//     type whose member type is compatible with the list's (B.1.2.1 a,
//     B.1.2.2 a), and is a specific value or a combined template, not a
//     matching mechanism as a whole (B.1.2.1 b, B.1.2.2 b).
//   - A template referenced in the list obeys the present restriction
//     (B.1.2.1 e, B.1.2.2 f): it is not omit or * as a whole.
//
// Types resolve through the root-type model; a rule fires only when the
// types it compares are known.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateListRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}

	var walk func(e syntax.Expr, t rootType, known bool)
	// element checks one element of a template list for type t.
	element := func(el syntax.Expr, t rootType, known bool, clause string) {
		if from, ok := el.(*syntax.FromExpr); ok && from.KindTok != nil && from.KindTok.Kind() == syntax.ALL {
			m.checkAllFromSource(from, t, known, clause, add)
			return
		}
		// A template referenced in the list; a literal omit or * is left
		// alone (an optional field's list may contain them).
		if body := m.templateBody(el); body != nil && isOmitOrAnyOrNone(body) {
			add(el, "template-list-not-present", fmt.Sprintf("%s in a template list: the list's templates obey the present restriction (ETSI %s)", el, clause))
			return
		}
		if known && t.primitive() {
			if r, ok := m.exprRoot(el); ok && r.primitive() && !assignable(t, r) {
				add(el, "template-list-element-type", fmt.Sprintf("%s is a %s, not a value of the list's type %s (ETSI %s)", el, r.kind, t.kind, clause))
				return
			}
		}
		walk(el, t, known)
	}
	walk = func(e syntax.Expr, t rootType, known bool) {
		switch x := e.(type) {
		case *syntax.ParenExpr:
			if len(x.List) == 1 && !isAllFrom(x.List[0]) {
				walk(x.List[0], t, known)
				return
			}
			for _, el := range x.List {
				element(el, t, known, "B.1.2.1")
			}
		case *syntax.CallExpr:
			if syntax.Name(x.Fun) == "complement" && x.Args != nil {
				for _, el := range x.Args.List {
					element(el, t, known, "B.1.2.2")
				}
			}
		case *syntax.CompositeLiteral:
			if !known || t.decl == nil {
				return
			}
			for _, item := range x.List {
				b, ok := item.(*syntax.BinaryExpr)
				if !ok || b.Op == nil || b.Op.Kind() != syntax.ASSIGN {
					continue
				}
				ft, ok := m.fieldRoot(t, syntax.Name(b.X))
				walk(b.Y, ft, ok)
			}
		}
	}

	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td.Type == nil || td.Value == nil {
			return true
		}
		t, known := m.typeRoot(tree, td.Type, 0)
		walk(td.Value, t, known)
		return true
	})
	return diags
}

func isAllFrom(e syntax.Expr) bool {
	from, ok := e.(*syntax.FromExpr)
	return ok && from.KindTok != nil && from.KindTok.Kind() == syntax.ALL
}

// isOmitOrAnyOrNone reports whether e is omit or * (AnyValueOrNone).
func isOmitOrAnyOrNone(e syntax.Expr) bool {
	v, ok := e.(*syntax.ValueLiteral)
	return ok && v.Tok != nil && (v.Tok.Kind() == syntax.MUL || v.Tok.Kind() == syntax.OMIT)
}

// fieldRoot is the root type of the field name of the record, set or
// union type t.
func (m typeModel) fieldRoot(t rootType, name string) (rootType, bool) {
	if t.decl == nil || name == "" {
		return rootType{}, false
	}
	for _, f := range t.decl.Fields {
		if f == nil || f.Name == nil || f.Name.String() != name || len(f.ArrayDef) > 0 {
			continue
		}
		if e := typeSpecExpr(f.Type); e != nil {
			return m.typeRoot(t.tree, e, 0)
		}
	}
	return rootType{}, false
}

// checkAllFromSource checks the template of `all from` in a list for type
// t (B.1.2.1 a, b; B.1.2.2 a, b).
func (m typeModel) checkAllFromSource(from *syntax.FromExpr, t rootType, known bool, clause string, add func(syntax.Node, string, string)) {
	id, ok := from.X.(*syntax.Ident)
	if !ok {
		return
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return
	}
	var typ, body syntax.Expr
	switch d := defs[0].Node.(type) {
	case *syntax.TemplateDecl:
		typ = d.Type
		if d.Params == nil && d.Base == nil {
			body = d.Value
		}
	default:
		// Variables, constants and parameters, arrays among them, are
		// left alone.
		return
	}
	src, ok := m.typeRoot(defs[0].Tree, typ, 0)
	if !ok {
		return
	}
	if src.kind != "record of" && src.kind != "set of" {
		add(from, "all-from-not-a-list", fmt.Sprintf("all from %s: %s is a %s template, not of a record of or set of type (ETSI %s)", id, id, src.kind, clause))
		return
	}
	if body != nil && isWholeMatchingMechanism(body) {
		add(from, "all-from-matching-mechanism", fmt.Sprintf("all from %s: %s is a matching mechanism as a whole, not a specific value or combined template (ETSI %s b)", id, id, clause))
		return
	}
	elem, ok := m.elemRootOfType(defs[0].Tree, typ)
	if !known || !ok {
		return
	}
	if sameFamily(t, elem) && !m.compatible(t, elem, 0) || t.primitive() != elem.primitive() {
		add(from, "all-from-member-type-incompatible", fmt.Sprintf("all from %s: its member type %s is not compatible with the list's type %s (ETSI %s a)", id, elem.kind, t.kind, clause))
	}
}

// isWholeMatchingMechanism reports whether a template body is, as a whole,
// a matching mechanism: ?, *, omit, a value list or a complemented list.
func isWholeMatchingMechanism(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		return x.Tok != nil && (x.Tok.Kind() == syntax.ANY || x.Tok.Kind() == syntax.MUL || x.Tok.Kind() == syntax.OMIT)
	case *syntax.ParenExpr:
		return len(x.List) > 1
	case *syntax.CallExpr:
		return syntax.Name(x.Fun) == "complement"
	}
	return false
}
