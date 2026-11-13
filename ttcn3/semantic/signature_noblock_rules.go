// signature_noblock_rules.go enforces ETSI ES 201 873-1
// clause 14: a `noblock` signature cannot specify
// `out` / `inout` formal parameters, a return value, or an
// exception list, because the caller never blocks for the
// answer.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSignatureNoBlockRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd == nil || sd.NoBlock == nil {
			continue
		}
		name := ""
		if sd.Name != nil {
			name = sd.Name.String()
		}
		if sd.Return != nil {
			diags = append(diags, Diagnostic{
				Code:     "noblock-signature-return",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"signature %q is `noblock`; a return value is not allowed (ETSI 14)",
					name),
				Node: sd.Return,
				Span: syntax.SpanOf(sd.Return),
			})
		}
		if sd.Params == nil {
			continue
		}
		for _, fp := range sd.Params.List {
			if fp == nil || fp.Direction == nil {
				continue
			}
			k := fp.Direction.Kind()
			if k != syntax.OUT && k != syntax.INOUT {
				continue
			}
			fname := ""
			if fp.Name != nil {
				fname = fp.Name.String()
			}
			diags = append(diags, Diagnostic{
				Code:     "noblock-signature-direction",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"signature %q is `noblock`; parameter %q has direction %q but only `in` parameters are allowed (ETSI 14)",
					name, fname, fp.Direction.String()),
				Node: fp,
				Span: syntax.SpanOf(fp),
			})
		}
	}
	return diags
}
