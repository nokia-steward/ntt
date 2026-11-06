// decoded_field_ref_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 7.2: when the extended syntax
// "x => (Type, encoding)" is used, the encoding parameter
// must be one of the strings allowed for the
// decvalue_unichar() function (clause C.5.4).
//
// We only check the easy, statically obvious case:
// the encoding operand must be a string literal whose
// value is in the canonical allow-list. Anything else
// (non-literal, computed, alias, etc.) is left for the
// runtime to validate.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkDecodedFieldRefRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil {
			return true
		}
		if be.Op.Kind() != syntax.DECODE {
			return true
		}
		pe, ok := be.Y.(*syntax.ParenExpr)
		if !ok || pe == nil || len(pe.List) < 2 {
			return true
		}
		enc := pe.List[len(pe.List)-1]
		vl, ok := enc.(*syntax.ValueLiteral)
		if !ok || vl == nil || vl.Tok == nil {
			return true
		}
		raw := vl.Tok.String()
		if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
			return true
		}
		value := raw[1 : len(raw)-1]
		if isValidUnicharEncoding(value) {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "decode-invalid-encoding-format",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"encoding format %q is not one of the strings allowed for decvalue_unichar (ETSI 7.2 / C.5.4)",
				value),
			Node: vl,
			Span: syntax.SpanOf(vl),
		})
		return true
	})
	return diags
}

func isValidUnicharEncoding(s string) bool {
	switch s {
	case "UTF-8", "UTF-16", "UTF-16LE", "UTF-16BE", "UTF-32", "UTF-32LE", "UTF-32BE":
		return true
	}
	return false
}
