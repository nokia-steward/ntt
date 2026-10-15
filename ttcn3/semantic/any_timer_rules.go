// any_timer_rules.go enforces ETSI ES 201 873-1 clause 23.7
// ("Summary of the use of any and all with timers"):
//
//	op       | any timer  | all timer
//	---------+------------+----------
//	start    | forbidden  | forbidden
//	stop     | forbidden  | allowed
//	read     | forbidden  | forbidden
//	running  | allowed    | forbidden
//	timeout  | allowed    | forbidden
//
// Selectors named `timeout`/`running` etc. against a normal timer
// receiver are untouched - that's covered by timer_arity_rules.go.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// anyTimerAllowedOps maps the `any|all` qualifier to the set of
// timer ops that are LEGAL with it. Anything else fires the
// `any-timer-forbidden-op` diag with a stable ETSI 23.7 reference.
var anyTimerAllowedOps = map[string]map[string]bool{
	"any": {
		"running": true,
		"timeout": true,
	},
	"all": {
		"stop": true,
	},
}

func (a *Analyzer) checkAnyTimerRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok || opIdent == nil {
			return true
		}
		qualifier := anyAllTimerQualifier(sel.X)
		if qualifier == "" {
			return true
		}
		op := opIdent.String()
		if anyTimerAllowedOps[qualifier][op] {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "any-timer-forbidden-op",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%q on `%s timer` is forbidden (ETSI 23.7)",
				op, qualifier),
			Node: sel,
			Span: syntax.SpanOf(sel),
		})
		return true
	})
	return diags
}

// anyAllTimerQualifier returns "any", "all", or "" depending on
// whether e is the qualifier portion of `any timer.X` /
// `all timer.X`. Mirrors isAnyAllTimerReceiver but exposes which
// keyword matched so we can apply per-qualifier op tables.
func anyAllTimerQualifier(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.Ident:
		if v == nil || v.Tok == nil || v.Tok2 == nil {
			return ""
		}
		first := v.Tok.String()
		second := v.Tok2.String()
		if (first == "any" || first == "all") && second == "timer" {
			return first
		}
	case *syntax.BinaryExpr:
		if v == nil {
			return ""
		}
		head, ok := v.X.(*syntax.Ident)
		if !ok || head == nil {
			return ""
		}
		s := head.String()
		if s != "any" && s != "all" {
			return ""
		}
		tail, ok := v.Y.(*syntax.Ident)
		if ok && tail != nil && tail.String() == "timer" {
			return s
		}
	}
	return ""
}

// isAnyAllTimerReceiver matches the `any timer` / `all timer`
// qualifier. The parser surfaces it as a single Ident with two
// tokens (Tok, Tok2). We also accept the two-Ident BinaryExpr
// fallback in case a future parser revision splits them.
func isAnyAllTimerReceiver(e syntax.Expr) bool {
	switch v := e.(type) {
	case *syntax.Ident:
		if v == nil || v.Tok == nil || v.Tok2 == nil {
			return false
		}
		first := v.Tok.String()
		second := v.Tok2.String()
		return (first == "any" || first == "all") && second == "timer"
	case *syntax.BinaryExpr:
		if v == nil {
			return false
		}
		head, ok := v.X.(*syntax.Ident)
		if !ok || head == nil {
			return false
		}
		s := head.String()
		if s != "any" && s != "all" {
			return false
		}
		tail, ok := v.Y.(*syntax.Ident)
		return ok && tail != nil && tail.String() == "timer"
	}
	return false
}
