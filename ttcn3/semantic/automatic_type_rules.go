// automatic_type_rules.go enforces ETSI ES 201 873-1 clause 6.5: a
// declaration without a type takes the type of what it is initialised
// with, which a matching symbol of no type, `?` or `*`, and `omit` do not
// have.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAutomaticTypes(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd.Type != nil {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Value == nil {
				continue
			}
			lit, ok := d.Value.(*syntax.ValueLiteral)
			if !ok || lit.Tok == nil {
				continue
			}
			switch lit.Tok.Kind() {
			case syntax.ANY, syntax.MUL, syntax.OMIT:
				diags = append(diags, Diagnostic{
					Code:     "automatic-type-unknown",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%q gives no type for %s declared without one (ETSI 6.5)", lit.Tok.String(), syntax.Name(d.Name)),
					Node:     d,
					Span:     syntax.SpanOf(d),
				})
			}
		}
		return true
	})
	return diags
}
