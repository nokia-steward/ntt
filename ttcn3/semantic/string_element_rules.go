// string_element_rules.go enforces ETSI ES 201 873-1 clause
// 6.1.1.1 / 6.1.0.1 on individual-element string access:
//
//   - `s[i] := value;` where `s` is a charstring / universal
//     charstring variable requires `value` to evaluate to a single
//     character (a string literal of length exactly 1). The
//     interpreter accepts any string and silently overwrites the
//     slot, so the negative fixture passes instead of rejects.
//
// We only flag literal-string RHS that the parser hands back as a
// STRING token. Any other shape (function returns, references,
// concatenations) falls through silently.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkStringElementRules walks every executable body looking
// for `varName[index] := stringLit;` shapes where varName is a
// known charstring / universal charstring local and the literal
// has length != 1.
func (a *Analyzer) checkStringElementRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		strVars := collectLocalStringVars(fn.Body)
		if len(strVars) == 0 {
			return false
		}
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			be, ok := sn.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			ie, ok := be.X.(*syntax.IndexExpr)
			if !ok || ie == nil {
				return true
			}
			varName := identName(ie.X)
			if varName == "" || !strVars[varName] {
				return true
			}
			if s, ok := stringLiteralValue(be.Y); ok {
				if runeCount(s) != 1 {
					diags = append(diags, Diagnostic{
						Code:     "string-element-not-single-char",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"individual string-element assignment to %q requires a single-character value (got %q, length %d)",
							varName, s, runeCount(s)),
						Node: be,
						Span: syntax.SpanOf(be),
					})
				}
			}
			return true
		})
		return false
	})
	return diags
}

// collectLocalStringVars returns a set of local variable names
// whose declared type is `charstring` or `universal charstring`.
func collectLocalStringVars(body syntax.Node) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := syntax.Name(vd.Type)
		if typeName != "charstring" && typeName != "universal charstring" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = true
		}
		return true
	})
	return out
}

// runeCount returns the number of unicode code points in s.
// We can't use len(s) because UTF-8 multibyte chars would
// inflate the count for universal charstring.
func runeCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}
