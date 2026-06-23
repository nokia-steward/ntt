// binary_literals.go implements ETSI ES 201 873-1 clause 6.1.1:
// bitstring (`'...'B`), hexstring (`'...'H`) and octetstring
// (`'...'O`) literals must contain only valid digits for the
// corresponding base. Wildcards (`?`, `*`) are also allowed in
// template / pattern positions but we accept them in literals
// regardless - the spec carves them out only in matching contexts.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkBinaryLiterals(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		v, ok := n.(*syntax.ValueLiteral)
		if !ok || v.Tok == nil {
			return true
		}
		if v.Tok.Kind() != syntax.BSTRING {
			return true
		}
		s := v.Tok.String()
		// Format: '<digits>'<suffix>  where suffix is B/H/O.
		if len(s) < 3 || s[0] != '\'' {
			return true
		}
		closeIdx := strings.LastIndexByte(s, '\'')
		if closeIdx <= 0 || closeIdx >= len(s)-1 {
			return true
		}
		body := s[1:closeIdx]
		suffix := strings.ToUpper(s[closeIdx+1:])
		var validate func(byte) bool
		var what string
		switch suffix {
		case "B":
			what = "bitstring"
			validate = func(c byte) bool {
				return c == '0' || c == '1' || c == '?' || c == '*'
			}
		case "H":
			what = "hexstring"
			validate = func(c byte) bool {
				return (c >= '0' && c <= '9') ||
					(c >= 'A' && c <= 'F') ||
					(c >= 'a' && c <= 'f') ||
					c == '?' || c == '*'
			}
		case "O":
			what = "octetstring"
			validate = func(c byte) bool {
				return (c >= '0' && c <= '9') ||
					(c >= 'A' && c <= 'F') ||
					(c >= 'a' && c <= 'f') ||
					c == '?' || c == '*'
			}
		default:
			return true
		}
		// `\` is a line-continuation marker the parser allows
		// inside literals - skip it together with whitespace.
		isSkip := func(c byte) bool {
			return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\\'
		}
		for _, c := range []byte(body) {
			if isSkip(c) {
				continue
			}
			if !validate(c) {
				diags = append(diags, Diagnostic{
					Code:     "invalid-binary-literal",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"invalid %s literal %q: contains forbidden character %q",
						what, s, string(c)),
					Node: v,
					Span: syntax.SpanOf(v),
				})
				return true
			}
		}
		// Octetstring length must be even (hex digits in
		// pairs). Wildcards `?` and `*` are not hex digits
		// and don't participate in the pairing rule.
		if suffix == "O" {
			digits := 0
			for _, c := range []byte(body) {
				if isSkip(c) || c == '?' || c == '*' {
					continue
				}
				digits++
			}
			if digits%2 != 0 {
				diags = append(diags, Diagnostic{
					Code:     "invalid-binary-literal",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"invalid octetstring literal %q: odd number of hex digits",
						s),
					Node: v,
					Span: syntax.SpanOf(v),
				})
			}
		}
		return true
	})
	return diags
}
