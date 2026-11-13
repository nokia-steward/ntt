// default_anytype_rules.go enforces ETSI ES 201 873-1 clause 6.2.5
// "The @default alternative shall not be of the anytype":
//
//   - In any record / set / union declaration, a field marked
//     `@default` whose declared type is `anytype` is rejected.
//
// The check is local to the field syntax. anytype is identified
// by its literal type-name; subtypes (`type anytype Alias;`) are
// not currently tracked but would be a future extension.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkDefaultAnytypeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st == nil {
			continue
		}
		for _, f := range st.Fields {
			if f == nil || f.DefaultTok == nil {
				continue
			}
			ref, ok := f.Type.(*syntax.RefSpec)
			if !ok || ref == nil {
				continue
			}
			if identName(ref.X) != "anytype" {
				continue
			}
			name := ""
			if f.Name != nil {
				name = f.Name.String()
			}
			diags = append(diags, Diagnostic{
				Code:     "default-anytype-field",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"@default field %q has type anytype; @default alternatives must not be of anytype (ETSI 6.2.5)",
					name),
				Node: f,
				Span: syntax.SpanOf(f),
			})
		}
	}
	return diags
}
