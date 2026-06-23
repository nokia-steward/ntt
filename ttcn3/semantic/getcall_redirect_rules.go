// getcall_redirect_rules.go enforces ETSI ES 201 873-1 clause
// 22.3.2 / 22.3.4: a `getcall` / `getreply` without a template
// matches "any call" and the receiver cannot redirect parameters
// (`-> param(...)`) or the return value (`-> value(...)`), because
// the matched signature shape is unknown.
//
// Catches NegSem_220302_GetcallOperation_003 and similar.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkGetcallRedirectRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		re, ok := n.(*syntax.RedirectExpr)
		if !ok || re == nil {
			return true
		}
		sel, ok := re.X.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			// Anything other than a bare `.getcall`
			// receiver carries an explicit template, so
			// the param/value shape is known.
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		op := opIdent.String()
		if op != "getcall" && op != "getreply" {
			return true
		}
		if len(re.Param) > 0 {
			diags = append(diags, Diagnostic{
				Code:     "getcall-anycall-param-redirect",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q without a template matches any call; `-> param(...)` is not allowed (ETSI 22.3.2 / 22.3.4)",
					op),
				Node: re,
				Span: syntax.SpanOf(re),
			})
		}
		if len(re.Value) > 0 && op == "getreply" {
			diags = append(diags, Diagnostic{
				Code:     "getreply-anyreply-value-redirect",
				Severity: SeverityError,
				Message: "`getreply` without a template matches any reply; `-> value(...)` is not allowed (ETSI 22.3.4)",
				Node:    re,
				Span:    syntax.SpanOf(re),
			})
		}
		return true
	})
	return diags
}
