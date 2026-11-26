// subtype_rules.go checks user-defined subtypes (ETSI ES 201 873-1 clause
// 6.2.13) and the values declared of them:
//
//   - a restriction following the name of a newly defined record of or set
//     of type constrains the innermost type, the elements (6.2.13.3);
//     the iterated type is a subset of the innermost type;
//   - a list subtype of a subtype lists values of the parent type only
//     (6.2.13.2);
//   - the templates of a list subtype of a record of, set of, union or
//     anytype type are complete; of a record or set type, those in value
//     list notation are (6.2.13.2);
//   - list and length subtyping are not mixed in the definition of a
//     structured subtype (6.2.13.4);
//   - a value of a subtype fulfils the constraints of the subtype and of
//     all its parent types (6.2.13.0).
//
// Values are compared only when both sides are literals in the same
// notation; anything else is not reported.
package semantic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSubtypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.SubTypeDecl:
			if x.Field != nil && x.Field.Name != nil && len(x.Field.ArrayDef) == 0 {
				m.checkSubtypeDecl(x.Field, add)
			}
		case *syntax.ValueDecl:
			if x.KindTok == nil || x.TemplateRestriction != nil || x.Type == nil {
				return true
			}
			switch x.KindTok.Kind() {
			case syntax.VAR, syntax.CONST, syntax.MODULEPAR:
			default:
				return true
			}
			for _, d := range x.Decls {
				if d != nil && d.Value != nil && len(d.ArrayDef) == 0 {
					m.checkSubtypeValue(m.tree, x.Type, d.Value, 0, add)
				}
			}
		}
		return true
	})
	return diags
}

// constrainedType is the type the constraints of the subtype definition f
// apply to, and whether they apply to its elements: the element type of a
// newly defined record of or set of type (6.2.13.3), else f's type.
func constrainedType(f *syntax.Field) (syntax.Expr, bool) {
	if ls, ok := f.Type.(*syntax.ListSpec); ok {
		return typeSpecExpr(ls.ElemType), true
	}
	return typeSpecExpr(f.Type), false
}

func (m typeModel) checkSubtypeDecl(f *syntax.Field, add func(syntax.Node, string, string)) {
	t, _ := constrainedType(f)
	if t == nil {
		return
	}
	root, known := m.typeRoot(m.tree, t, 0)
	structured := known && root.kind != "enumerated" && root.structured()
	if f.ValueConstraint != nil && f.LengthConstraint != nil && structured {
		add(f.LengthConstraint, "subtype-mixed-mechanisms", fmt.Sprintf(
			"subtype %s of the %s type %s mixes list and length subtyping (ETSI 6.2.13.4)", f.Name, root.kind, sourceText(t)))
	}
	if f.ValueConstraint == nil {
		return
	}
	if structured {
		for _, e := range f.ValueConstraint.List {
			if bad := partialListTemplate(e, root.kind); bad != nil {
				add(bad, "subtype-list-partial-template", fmt.Sprintf(
					"subtype %s: the template %s of the list is not a complete template of the %s type (ETSI 6.2.13.2)", f.Name, sourceText(e), root.kind))
			}
		}
	}
	// The listed values are values of the parent type (6.2.13.2), and
	// the bounds of a range lie in it (6.2.13.3).
	parent, tree := m.subtypeField(m.tree, t)
	for depth := 0; parent != nil && depth < 8; depth++ {
		pt, elems := constrainedType(parent)
		if elems {
			break
		}
		if parent.ValueConstraint != nil {
			// Numeric values are checked against ranges and
			// lists alike.
			if root.kind == "integer" || root.kind == "float" {
				m.checkRangeInParent(f, parent, root.kind, add)
			} else {
				m.checkListInParent(f, parent, known && root.kind != "set of", add)
			}
		}
		parent, tree = m.subtypeField(tree, pt)
	}
}

// checkListInParent reports the values listed by f that are not listed by
// the parent subtype definition p, both lists of literals.
func (m typeModel) checkListInParent(f, p *syntax.Field, ordered bool, add func(syntax.Node, string, string)) {
	keys := map[string]bool{}
	shape := ""
	for i, e := range p.ValueConstraint.List {
		k, sh, ok := m.literalKey(e, ordered)
		if !ok || i > 0 && sh != shape {
			return
		}
		keys[k], shape = true, sh
	}
	for _, e := range f.ValueConstraint.List {
		k, sh, ok := m.literalKey(e, ordered)
		if !ok || sh != shape {
			continue
		}
		if !keys[k] {
			add(e, "subtype-value-not-in-parent", fmt.Sprintf(
				"subtype %s: %s is not a value of the parent type %s (ETSI 6.2.13.2)", f.Name, sourceText(e), p.Name))
		}
	}
}

// checkRangeInParent reports the range bounds and values of the numeric
// subtype definition f outside the constraint of the parent p.
func (m typeModel) checkRangeInParent(f, p *syntax.Field, kind string, add func(syntax.Node, string, string)) {
	if kind != "integer" && kind != "float" {
		return
	}
	ps, ok := parseValueConstraint(p.ValueConstraint, kind)
	if !ok {
		return
	}
	for _, e := range f.ValueConstraint.List {
		var bounds []syntax.Expr
		if b, ok := e.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.RANGE {
			bounds = []syntax.Expr{b.X, b.Y}
		} else {
			bounds = []syntax.Expr{e}
		}
		for _, x := range bounds {
			if _, exc := stripExcl(x); exc {
				continue
			}
			if v, ok := numericValue(x); ok && !ps.includes(v) {
				add(e, "subtype-not-a-subset", fmt.Sprintf(
					"subtype %s: %s is not a value of the parent type %s (ETSI 6.2.13.3, 6.2.13.1)", f.Name, sourceText(x), p.Name))
				break
			}
		}
	}
}

// checkSubtypeValue reports where the literal value v of type t breaks a
// constraint of t or of one of its parent types (6.2.13.0), and, for
// the elements of a record of or set of value, of the element type.
func (m typeModel) checkSubtypeValue(tree *ttcn3.Tree, t syntax.Expr, v syntax.Expr, depth int, add func(syntax.Node, string, string)) {
	if depth > 8 {
		return
	}
	f, tree := m.subtypeField(tree, t)
	for i := 0; f != nil && i < 8; i++ {
		ct, elems := constrainedType(f)
		if elems {
			cl, ok := v.(*syntax.CompositeLiteral)
			if !ok {
				return
			}
			for _, e := range cl.List {
				if isAssignNotation(e) {
					return
				}
			}
			for _, e := range cl.List {
				if f.LengthConstraint != nil {
					m.checkValueLength(f, f.LengthConstraint, e, add)
				}
				m.checkSubtypeValue(tree, ct, e, depth+1, add)
			}
			return
		}
		if f.LengthConstraint != nil {
			if _, ok := v.(*syntax.CompositeLiteral); ok {
				m.checkValueLength(f, f.LengthConstraint, v, add)
			}
		}
		if f.ValueConstraint != nil {
			if _, ok := v.(*syntax.CompositeLiteral); ok {
				m.checkValueInList(tree, f, v, add)
			}
		}
		f, tree = m.subtypeField(tree, ct)
	}
}

// checkValueLength reports the literal value v when its number of
// elements or characters lies outside the length restriction le of the
// subtype f.
func (m typeModel) checkValueLength(f *syntax.Field, le *syntax.LengthExpr, v syntax.Expr, add func(syntax.Node, string, string)) {
	spec, ok := parseLengthExpr(le, "")
	if !ok {
		return
	}
	var n int
	switch x := v.(type) {
	case *syntax.CompositeLiteral:
		for _, e := range x.List {
			if isAssignNotation(e) {
				return
			}
		}
		n = len(x.List)
	case *syntax.ValueLiteral:
		if n, _, ok = stringLiteralLength(x); !ok {
			return
		}
	default:
		return
	}
	if !spec.includes(n) {
		add(v, "subtype-length-violation", fmt.Sprintf(
			"%s has %d elements, outside %s of the type %s (ETSI 6.2.13)", sourceText(v), n, spec.describe(), f.Name))
	}
}

// checkValueInList reports the literal value v when the list of literals
// of the subtype f does not hold it.
func (m typeModel) checkValueInList(tree *ttcn3.Tree, f *syntax.Field, v syntax.Expr, add func(syntax.Node, string, string)) {
	t, _ := constrainedType(f)
	root, known := m.typeRoot(tree, t, 0)
	ordered := known && root.kind != "set of"
	k, shape, ok := m.literalKey(v, ordered)
	if !ok {
		return
	}
	for _, e := range f.ValueConstraint.List {
		pk, sh, ok := m.literalKey(e, ordered)
		if !ok || sh != shape {
			return
		}
		if pk == k {
			return
		}
	}
	add(v, "subtype-value-not-in-list", fmt.Sprintf(
		"%s is not one of the values of the type %s (ETSI 6.2.13.0)", sourceText(v), f.Name))
}

// subtypeField is the definition of the subtype t names, and the tree it
// is in; nil when t names no subtype.
func (m typeModel) subtypeField(tree *ttcn3.Tree, t syntax.Expr) (*syntax.Field, *ttcn3.Tree) {
	if tree == nil || t == nil {
		return nil, nil
	}
	if _, ok := t.(*syntax.Ident); !ok {
		if _, ok := t.(*syntax.SelectorExpr); !ok {
			return nil, nil
		}
	}
	defs := tree.LookupWithDB(t, m.db)
	if len(defs) != 1 {
		return nil, nil
	}
	var f *syntax.Field
	switch d := defs[0].Node.(type) {
	case *syntax.SubTypeDecl:
		f = d.Field
	case *syntax.Field:
		if _, ok := defs[0].ParentOf(d).(*syntax.SubTypeDecl); ok {
			f = d
		}
	}
	if f == nil || f.Name == nil || len(f.ArrayDef) > 0 {
		return nil, nil
	}
	return f, defs[0].Tree
}

// partialListTemplate is the uninitialised element of the template e of a
// list subtype of a type of root kind, or nil: any of a record of, set
// of, union or anytype value; one in value list notation of a record or
// set value (6.2.13.2).
func partialListTemplate(e syntax.Expr, kind string) syntax.Expr {
	cl, ok := e.(*syntax.CompositeLiteral)
	if !ok {
		return nil
	}
	deep := kind != "record" && kind != "set"
	for _, x := range cl.List {
		if isAssignNotation(x) {
			if b := x.(*syntax.BinaryExpr); deep {
				if bad := partialListTemplate(b.Y, ""); bad != nil {
					return bad
				}
				if isNotUsedSymbol(b.Y) {
					return b.Y
				}
			}
			continue
		}
		if isNotUsedSymbol(x) {
			return x
		}
		if deep {
			if bad := partialListTemplate(x, ""); bad != nil {
				return bad
			}
		}
	}
	return nil
}

func isAssignNotation(e syntax.Expr) bool {
	b, ok := e.(*syntax.BinaryExpr)
	return ok && b.Op != nil && b.Op.Kind() == syntax.ASSIGN
}

// literalKey is a canonical text of the literal value e, for comparing
// values written in the same notation, and that notation: scalar
// literals, enumerated values, booleans and omit, and brace-enclosed
// lists and field assignments of them; ok false for anything else, such
// as a reference to a constant. A list is compared in order when ordered
// is set; nested lists, whose type may be a set of, never are, which can
// only miss an error.
func (m typeModel) literalKey(e syntax.Expr, ordered bool) (key, shape string, ok bool) {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil {
			return "", "", false
		}
		switch x.Tok.Kind() {
		case syntax.INT, syntax.FLOAT, syntax.STRING, syntax.BSTRING, syntax.OMIT:
			return x.Tok.String(), "", true
		}
	case *syntax.Ident:
		switch x.String() {
		case "true", "false", "omit":
			return x.String(), "", true
		}
		// An enumerated value resolves to nothing else.
		for _, d := range m.tree.LookupWithDB(x, m.db) {
			switch d.Node.(type) {
			case *syntax.EnumTypeDecl, *syntax.EnumSpec:
			default:
				return "", "", false
			}
		}
		return x.String(), "", true
	case *syntax.CompositeLiteral:
		// The shape of a list is that of its elements, which share
		// one; of field assignments, the fields and their shapes.
		var parts, fields []string
		elem := ""
		assign := len(x.List) > 0 && isAssignNotation(x.List[0])
		for i, y := range x.List {
			if isAssignNotation(y) != assign {
				return "", "", false
			}
			if assign {
				b := y.(*syntax.BinaryExpr)
				name, ok := b.X.(*syntax.Ident)
				if !ok {
					return "", "", false
				}
				k, sh, ok := m.literalKey(b.Y, false)
				if !ok {
					return "", "", false
				}
				parts = append(parts, name.String()+"="+k)
				fields = append(fields, name.String()+":"+sh)
				continue
			}
			k, sh, ok := m.literalKey(y, false)
			if !ok || i > 0 && sh != elem {
				return "", "", false
			}
			parts, elem = append(parts, k), sh
		}
		if assign || !ordered {
			sort.Strings(parts)
		}
		if assign {
			sort.Strings(fields)
			return "{" + strings.Join(parts, ",") + "}", "{" + strings.Join(fields, ",") + "}", true
		}
		return "[" + strings.Join(parts, ",") + "]", "[" + elem + "]", true
	}
	return "", "", false
}
