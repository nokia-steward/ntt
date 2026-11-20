// recursive_type_rules.go enforces ETSI ES 201 873-1 clause 6.2
// restrictions on self-referential structured types:
//
//   - a `union` type must have at least one alternative that does
//     NOT reference the union itself (NegSyn_0602_TopLevel_001);
//   - a `record` / `set` type must mark every self-referential
//     field as `optional` to avoid infinite recursion
//     (NegSyn_0602_TopLevel_002).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRecursiveTypeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil || st.KindTok == nil {
			continue
		}
		typeName := st.Name.String()
		kind := st.KindTok.String()
		switch kind {
		case "union":
			if len(st.Fields) == 0 {
				continue
			}
			allSelf := true
			for _, f := range st.Fields {
				if !fieldReferencesType(f, typeName) {
					allSelf = false
					break
				}
			}
			if allSelf {
				diags = append(diags, Diagnostic{
					Code:     "recursive-union-no-base",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"union %q has only self-referential alternatives (ETSI 6.2)",
						typeName),
					Node: st.Name,
					Span: syntax.SpanOf(st.Name),
				})
			}
		case "record", "set":
			for _, f := range st.Fields {
				if !fieldReferencesType(f, typeName) {
					continue
				}
				if f.Optional != nil {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "recursive-record-field-non-optional",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s %q has a non-optional field of its own type (ETSI 6.2)",
						kind, typeName),
					Node: f.Name,
					Span: syntax.SpanOf(f.Name),
				})
			}
		}
	}
	return diags
}

// fieldReferencesType returns true when the field's declared
// type is exactly `typeName`. We only handle bare RefSpec /
// Ident shapes; anything more elaborate (TypePars, qualified
// names, ...) is conservatively treated as non-matching.
func fieldReferencesType(f *syntax.Field, typeName string) bool {
	if f == nil {
		return false
	}
	ref, ok := f.Type.(*syntax.RefSpec)
	if !ok || ref == nil || ref.X == nil {
		return false
	}
	id, ok := ref.X.(*syntax.Ident)
	if !ok || id == nil {
		return false
	}
	return id.String() == typeName
}
