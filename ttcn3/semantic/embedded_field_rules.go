// embedded_field_rules.go enforces ETSI ES 201 873-1 clause 6.2.1.4: an
// embedded field of a record or set is named by its type; the fields of
// the embedded type are not fields of the embedding one, so assignment
// notation cannot name them.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkEmbeddedFieldNames(mod *syntax.Module) []Diagnostic {
	// The fields of each record or set type of mod that embeds a field.
	embedding := map[string]map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		st, ok := n.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil || st.KindTok == nil || st.KindTok.Kind() == syntax.UNION {
			return true
		}
		fields, embeds := map[string]bool{}, false
		for _, f := range st.Fields {
			if f == nil {
				continue
			}
			switch {
			case f.Name != nil:
				fields[f.Name.String()] = true
			case f.EmbeddedName() != nil:
				fields[f.EmbeddedName().String()] = true
				embeds = true
			}
		}
		if embeds {
			embedding[st.Name.String()] = fields
		}
		return true
	})
	if len(embedding) == 0 {
		return nil
	}
	var diags []Diagnostic
	check := func(typ syntax.Expr, val syntax.Expr) {
		id, ok := typ.(*syntax.Ident)
		if !ok {
			return
		}
		fields, ok := embedding[id.String()]
		if !ok {
			return
		}
		cl, ok := val.(*syntax.CompositeLiteral)
		if !ok || !isAllNamedAssign(cl) {
			return
		}
		for _, e := range cl.List {
			name := syntax.Name(e.(*syntax.BinaryExpr).X)
			if !fields[name] {
				diags = append(diags, Diagnostic{
					Code:     "embedded-field-name",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s has no field %s; an embedded field is named by its type (ETSI 6.2.1.4)", id.String(), name),
					Node:     e,
					Span:     syntax.SpanOf(e),
				})
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			for _, d := range x.Decls {
				if d != nil {
					check(x.Type, d.Value)
				}
			}
		case *syntax.TemplateDecl:
			check(x.Type, x.Value)
		}
		return true
	})
	return diags
}
