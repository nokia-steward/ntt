// map_key_char_rules.go reports an index of a map whose from type is
// charstring by a universal charstring key with a character charstring
// does not have (ETSI ES 201 873-1 clauses 6.3.1, 6.3.8): a map of
// universal charstring keys is compatible with one of charstring keys,
// but its values for keys that are not charstring values cannot be
// accessed through it.
//
// Only keys whose characters are known are checked: char(g, p, r, c)
// quadruples, string literals and constants initialised with either.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkMapKeyCharRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		x, ok := n.(*syntax.IndexExpr)
		if !ok {
			return true
		}
		id, ok := x.X.(*syntax.Ident)
		if !ok {
			return true
		}
		from, ok := m.mapFromRoot(id)
		if !ok || from.kind != "charstring" {
			return true
		}
		if m.hasNonCharstringChar(x.Index) {
			diags = append(diags, Diagnostic{
				Code:     "map-key-not-charstring",
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s[%s]: the key has a character charstring does not have, and %s's keys are charstring values (ETSI 6.3.1, 6.3.8)", id, x.Index, id),
				Node:     x.Index,
				Span:     syntax.SpanOf(x.Index),
			})
		}
		return true
	})
	return diags
}

// mapFromRoot is the root type of the from type of the map value id
// refers to.
func (m typeModel) mapFromRoot(id *syntax.Ident) (rootType, bool) {
	vd, decl, def, ok := m.valueDeclOf(id)
	if !ok || vd.Type == nil || len(decl.ArrayDef) > 0 {
		return rootType{}, false
	}
	defs := def.Tree.LookupWithDB(vd.Type, m.db)
	if len(defs) != 1 {
		return rootType{}, false
	}
	var spec *syntax.MapSpec
	switch d := defs[0].Node.(type) {
	case *syntax.MapTypeDecl:
		spec = d.Spec
	case *syntax.SubTypeDecl:
		if d.Field != nil {
			spec, _ = d.Field.Type.(*syntax.MapSpec)
		}
	case *syntax.Field:
		spec, _ = d.Type.(*syntax.MapSpec)
	}
	if spec == nil {
		return rootType{}, false
	}
	e := typeSpecExpr(spec.FromType)
	if e == nil {
		return rootType{}, false
	}
	return m.typeRoot(defs[0].Tree, e, 0)
}

// hasNonCharstringChar reports whether the key e is known to have a
// character outside charstring's, code points 0 to 127 (6.1.1).
func (m typeModel) hasNonCharstringChar(e syntax.Expr) bool {
	if id, ok := e.(*syntax.Ident); ok {
		vd, decl, _, ok := m.valueDeclOf(id)
		if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.CONST || decl.Value == nil {
			return false
		}
		e = decl.Value
	}
	switch x := e.(type) {
	case *syntax.CallExpr:
		if syntax.Name(x.Fun) != "char" || x.Args == nil || len(x.Args.List) != 4 {
			return false
		}
		var q [4]int64
		for i, arg := range x.Args.List {
			n, ok := signedIntLiteral(arg)
			if !ok {
				return false
			}
			q[i] = n
		}
		return q[0] != 0 || q[1] != 0 || q[2] != 0 || q[3] > 127
	case *syntax.ValueLiteral:
		if x.Tok == nil || x.Tok.Kind() != syntax.STRING {
			return false
		}
		for _, r := range x.Tok.String() {
			if r > 127 {
				return true
			}
		}
	}
	return false
}
