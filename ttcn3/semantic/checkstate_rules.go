// checkstate_rules.go enforces ETSI ES 201 873-1 clause 22.5.5
// restriction a: the argument of checkstate is a charstring naming one
// of six port states. A literal argument is checked here; any other is
// checked when it is evaluated (restriction b).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkstateStates are the states checkstate takes, compared exactly.
var checkstateStates = map[string]bool{
	"Started": true, "Halted": true, "Stopped": true,
	"Connected": true, "Mapped": true, "Linked": true,
}

func (a *Analyzer) checkCheckstateArgs(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce.Args == nil || len(ce.Args.List) != 1 {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || syntax.Name(sel.Sel) != "checkstate" {
			return true
		}
		lit, ok := ce.Args.List[0].(*syntax.ValueLiteral)
		if !ok || lit.Tok == nil {
			return true
		}
		var reason string
		if s, ok := stringLiteralValue(lit); !ok {
			reason = fmt.Sprintf("%s is not a charstring", lit.Tok.String())
		} else if !checkstateStates[s] {
			reason = fmt.Sprintf("%q is not one of Started, Halted, Stopped, Connected, Mapped, Linked", s)
		} else {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "checkstate-invalid-state",
			Severity: SeverityError,
			Message:  fmt.Sprintf("checkstate: %s (ETSI 22.5.5)", reason),
			Node:     lit,
			Span:     syntax.SpanOf(lit),
		})
		return true
	})
	return diags
}
