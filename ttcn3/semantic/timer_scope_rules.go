package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkTimerScopeRules enforces ETSI 23 introductory clause: timer
// operations are allowed inside test cases, functions, altsteps
// and module control - NOT in component-body initialisers. The
// canonical violations:
//
//	type component C {
//	    timer t1 := 1.0;
//	    timer t2 := t1.read;          // read inside component body
//	    var boolean v := t1.running;  // running inside component body
//	}
//
// We walk every ComponentTypeDecl, then walk its body looking for
// timer observation operations on any selector with a Sel ident
// matching `read`/`running`/`timeout`. The receiver doesn't need
// to be a verified timer because the initialiser context already
// guarantees the rule applies.
func (a *Analyzer) checkTimerScopeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ctd, ok := n.(*syntax.ComponentTypeDecl)
		if !ok || ctd == nil || ctd.Body == nil {
			return true
		}
		syntax.Inspect(ctd.Body, func(c syntax.Node) bool {
			sel, ok := c.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			opIdent, ok := sel.Sel.(*syntax.Ident)
			if !ok || opIdent == nil {
				return true
			}
			op := opIdent.String()
			if !timerObservationOp(op) {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "timer-op-in-component-body",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"timer operation `%s` is not allowed inside a component body; move to a testcase / function / altstep / control (ETSI 23)",
					op),
				Node: sel,
				Span: syntax.SpanOf(sel),
			})
			return true
		})
		return true
	})
	return diags
}
