// signature_data_type_rules.go enforces ETSI ES 201 873-1 clause 14
// restriction b: signature parameters and the return type shall be of a
// data type. Timer, default, component and port types hold object
// references, so they are not data types.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSignatureDataTypeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	classify := nonDataTypeKinds(mod)
	var diags []Diagnostic
	report := func(sig string, what string, te syntax.Expr) {
		name := identNameForPortRule(te)
		kind := classify(name)
		if kind == "" {
			return
		}
		diags = append(diags, Diagnostic{
			Code:     "signature-non-data-type",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s of signature %q is of %s type %q; expected a data type (ETSI 14 b)",
				what, sig, kind, name),
			Node: te,
			Span: syntax.SpanOf(te),
		})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sd, ok := n.(*syntax.SignatureDecl)
		if !ok || sd == nil {
			return true
		}
		sig := syntax.Name(sd.Name)
		if sd.Params != nil {
			for _, fp := range sd.Params.List {
				if fp != nil && fp.Type != nil {
					report(sig, fmt.Sprintf("parameter %q", syntax.Name(fp.Name)), fp.Type)
				}
			}
		}
		if sd.Return != nil && sd.Return.Type != nil {
			report(sig, "the return type", sd.Return.Type)
		}
		return false
	})
	return diags
}
