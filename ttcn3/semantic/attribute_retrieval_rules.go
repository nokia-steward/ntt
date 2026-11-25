// attribute_retrieval_rules.go checks the operations retrieving
// attribute values and the operand of setencode (ETSI ES 201 873-1
// clauses 27.8, 27.9):
//
//   - only variant takes a parameter (27.8 a), a universal charstring
//     (27.8 b) naming one of the encode attributes of the type (27.8 c);
//   - setencode applies to a port (27.9 c).
//
// The encode attributes of 27.8 c are known for types of this module
// that have encode attributes of their own, outside groups; a parameter
// is checked against them, and the module's, when it is a string
// literal.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/attr"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAttributeRetrievalRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	encodes := typeEncodes(mod)
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || call.Args == nil {
			return true
		}
		sel, ok := call.Fun.(*syntax.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		op := syntax.Name(sel.Sel)
		switch op {
		case "setencode":
			if r, ok := m.exprRoot(sel.X); ok && (r.primitive() || r.structured()) {
				add(sel.X, "setencode-not-a-port", fmt.Sprintf("%s.setencode: %s is of type %s, not a port type (ETSI 27.9 c)", tokenText(sel.X), tokenText(sel.X), r.kind))
			}
		case "encode", "display", "extension", "optional":
			if attributeOperation(sel.Sel) && len(call.Args.List) > 0 {
				add(call.Args, "attribute-parameter-not-variant", fmt.Sprintf("%s.%s: only variant takes a parameter (ETSI 27.8 a)", tokenText(sel.X), op))
			}
		case "variant":
			if !attributeOperation(sel.Sel) || len(call.Args.List) != 1 {
				return true
			}
			arg := call.Args.List[0]
			if r, ok := m.exprRoot(arg); ok && r.primitive() && !isCharKind(r.kind) {
				add(arg, "attribute-parameter-type", fmt.Sprintf("%s.variant: the parameter is of root type %s, not universal charstring (ETSI 27.8 b)", tokenText(sel.X), r.kind))
				return true
			}
			lit, ok := arg.(*syntax.ValueLiteral)
			if !ok || lit.Tok == nil || lit.Tok.Kind() != syntax.STRING {
				return true
			}
			valid, ok := encodes[m.attributeTypeDecl(sel.X)]
			if !ok {
				return true
			}
			name := unquote(lit.Tok.String())
			if !valid[name] {
				add(arg, "attribute-parameter-unknown-encoding", fmt.Sprintf("%s.variant(%s): %q is not an encode attribute of the type (ETSI 27.8 c)", tokenText(sel.X), lit, name))
			}
		}
		return true
	})
	return diags
}

// attributeOperation reports whether the selector sel is the keyword of
// an attribute operation, not a field of the same name.
func attributeOperation(sel syntax.Expr) bool {
	id, ok := sel.(*syntax.Ident)
	if !ok || id.Tok == nil {
		return false
	}
	switch id.Tok.Kind() {
	case syntax.ENCODE, syntax.DISPLAY, syntax.EXTENSION, syntax.OPTIONAL, syntax.VARIANT:
		return true
	}
	return false
}

// typeEncodes maps the type declarations of mod that have encode
// attributes of their own, outside groups, to their encode attributes
// and the module's. A type with an encode attribute for a field only is
// left out.
func typeEncodes(mod *syntax.Module) map[syntax.Node]map[string]bool {
	moduleEncodes, ok := encodeValues(mod.With)
	if !ok {
		return nil
	}
	out := map[syntax.Node]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		// The key is the node a reference to the type resolves to: the
		// Field of a SubTypeDecl.
		var with *syntax.WithSpec
		key := d.Def
		switch t := d.Def.(type) {
		case *syntax.SubTypeDecl:
			if t.Field == nil {
				continue
			}
			with, key = t.With, t.Field
		case *syntax.StructTypeDecl:
			with = t.With
		case *syntax.EnumTypeDecl:
			with = t.With
		case *syntax.MapTypeDecl:
			with = t.With
		default:
			continue
		}
		own, ok := encodeValues(with)
		if !ok || len(own) == 0 {
			continue
		}
		for k := range moduleEncodes {
			own[k] = true
		}
		out[key] = own
	}
	return out
}

// encodeValues are the encode attributes of ws, ok false when one of them
// applies to a field only.
func encodeValues(ws *syntax.WithSpec) (map[string]bool, bool) {
	vals := map[string]bool{}
	if ws == nil {
		return vals, true
	}
	set, _ := attr.Parse(ws)
	for _, at := range set.Attributes {
		if at.Kind != attr.Encode {
			continue
		}
		if len(at.Selectors) > 0 {
			return nil, false
		}
		vals[at.Value] = true
	}
	return vals, true
}

// attributeTypeDecl is the type declaration x names, or of the value x
// refers to; nil when unknown.
func (m typeModel) attributeTypeDecl(x syntax.Expr) syntax.Node {
	id, ok := x.(*syntax.Ident)
	if !ok {
		return nil
	}
	tree := m.tree
	var t syntax.Expr = id
	if typ, tr := m.declType(id); typ != nil {
		t, tree = typ, tr
	}
	defs := tree.LookupWithDB(t, m.db)
	if len(defs) != 1 {
		return nil
	}
	return defs[0].Node
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
