// template_attribute.go checks the template attribute of TTCN-3 V5 (ETSI
// TR 104 081 6.13): "Only modules can have template attribute", and its
// value is one of "static" and "dynamic".
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateAttribute(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ws, ok := n.(*syntax.WithSpec)
		if !ok {
			return true
		}
		for _, stmt := range ws.List {
			if stmt == nil || stmt.KindTok == nil || stmt.KindTok.Kind() != syntax.TEMPLATE {
				continue
			}
			switch v := stringLiteralOf(stmt.Value); {
			case ws != mod.With:
				diags = append(diags, Diagnostic{
					Code:     "template-attribute-not-on-module",
					Severity: SeverityError,
					Message:  "only a module has a template attribute (ETSI TR 104 081 6.13)",
					Node:     stmt,
					Span:     syntax.SpanOf(stmt),
				})
			case v != "static" && v != "dynamic":
				diags = append(diags, Diagnostic{
					Code:     "template-attribute-value",
					Severity: SeverityError,
					Message:  fmt.Sprintf("template attribute %q is neither \"static\" nor \"dynamic\" (ETSI TR 104 081 6.13)", v),
					Node:     stmt,
					Span:     syntax.SpanOf(stmt),
				})
			}
		}
		return true
	})
	return diags
}
