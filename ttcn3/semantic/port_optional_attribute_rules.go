// port_optional_attribute_rules.go enforces ETSI ES 201
// 873-1 clause 27.7.a: a port type shall not have an
// \`optional\` attribute associated to it directly via a
// \`with\` clause. The optional attribute is only meaningful
// on structured types (record, set, ...).
//
// Example that is rejected:
//
//	type port loopbackPort message {
//	    inout MessageType
//	} with { optional "implicit omit" }   // ← error
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPortOptionalAttributeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ptd, ok := d.Def.(*syntax.PortTypeDecl)
		if !ok || ptd == nil || ptd.With == nil {
			continue
		}
		for _, w := range ptd.With.List {
			if w == nil || w.KindTok == nil {
				continue
			}
			if w.KindTok.Kind() != syntax.OPTIONAL {
				continue
			}
			name := ""
			if ptd.Name != nil {
				name = ptd.Name.String()
			}
			diags = append(diags, Diagnostic{
				Code:     "port-optional-attribute-forbidden",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"port type %q has an `optional` attribute; that attribute is not allowed on port types (ETSI 27.7.a)",
					name),
				Node: w,
				Span: syntax.SpanOf(w),
			})
		}
	}
	return diags
}
