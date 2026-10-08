// port_type_rules.go enforces ETSI ES 201 873-1 clause 6.2.9:
// every message / procedure / mixed port type declaration must
// list at least one direction (in / out / inout). The conformance
// suite covers this via NegSem_060209_CommunicationPortTypes_004
// (message) and _005 (procedure).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPortTypeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		pt, ok := d.Def.(*syntax.PortTypeDecl)
		if !ok || pt.Name == nil {
			continue
		}
		hasDir := false
		for _, attr := range pt.Attrs {
			pa, ok := attr.(*syntax.PortAttribute)
			if !ok || pa == nil || pa.KindTok == nil {
				continue
			}
			switch pa.KindTok.Kind() {
			case syntax.IN, syntax.OUT, syntax.INOUT:
				hasDir = true
			}
			if hasDir {
				break
			}
		}
		if hasDir {
			continue
		}
		kindLabel := "message"
		if pt.KindTok != nil {
			kindLabel = pt.KindTok.String()
		}
		diags = append(diags, Diagnostic{
			Code:     "port-type-no-direction",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s port type %q has no in/out/inout list (ETSI 6.2.9)",
				kindLabel, pt.Name.String()),
			Node: pt.Name,
			Span: syntax.SpanOf(pt.Name),
		})
	}
	return diags
}
