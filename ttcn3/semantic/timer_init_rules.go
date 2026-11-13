// timer_init_rules.go enforces ETSI ES 201 873-1 clause 12
// "Declaring timers": when a timer declaration carries a
// default duration it shall be a non-negative float literal.
// Notably,
//
//	timer t_timer := -1.0;
//
// is rejected because a timer duration must be >= 0.0 (the
// language doesn't support negative timer durations).
//
// We only flag the case where the right-hand side is a
// directly visible negative literal (a UnaryExpr with a
// minus operator over a numeric ValueLiteral). Expressions
// that compute the duration at runtime are left to the
// runtime semantic check; this rule's job is the trivially
// static "minus literal" case the conformance suite cares
// about.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTimerInitRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		if !isTimerValueDecl(vd) {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Value == nil {
				continue
			}
			name := ""
			if dc.Name != nil {
				name = dc.Name.String()
			}
			// Timer arrays (`timer t[N] := { ... }`) carry a
			// value-list initialiser; validate it element-wise
			// instead of treating the whole `{...}` as a scalar
			// default duration.
			if len(dc.ArrayDef) > 0 {
				diags = append(diags, checkTimerArrayInit(dc, name)...)
				continue
			}
			if lit, ok := negativeNumericLiteral(dc.Value); ok {
				diags = append(diags, Diagnostic{
					Code:     "timer-negative-default",
					Severity: SeverityError,
					Message: "timer " + quoted(name) +
						": default duration " + lit +
						" must be non-negative (ETSI 12)",
					Node: dc,
					Span: syntax.SpanOf(dc),
				})
				continue
			}
			if integerOnlyLiteral(dc.Value) {
				diags = append(diags, Diagnostic{
					Code:     "timer-non-float-default",
					Severity: SeverityError,
					Message: "timer " + quoted(name) +
						": default duration must be a float literal (e.g. 1.0), not an integer (ETSI 12)",
					Node: dc,
					Span: syntax.SpanOf(dc),
				})
			}
		}
		return true
	})
	return diags
}

// checkTimerArrayInit validates the initialiser of a timer array
// declarator `timer t[N] := <init>` against ETSI clause 12:
//
//   - the initialiser must be a value list `{ ... }`, not a single
//     value (`timer t[4] := 1.0` is rejected);
//   - the number of elements must match the declared dimension N;
//   - no element may be a negative duration literal.
func checkTimerArrayInit(dc *syntax.Declarator, name string) []Diagnostic {
	var diags []Diagnostic
	cl, ok := dc.Value.(*syntax.CompositeLiteral)
	if !ok {
		diags = append(diags, Diagnostic{
			Code:     "timer-array-non-list-init",
			Severity: SeverityError,
			Message: "timer array " + quoted(name) +
				": an array of timers must be initialised with a value list { ... }, not a single value (ETSI 12)",
			Node: dc,
			Span: syntax.SpanOf(dc),
		})
		return diags
	}
	if len(dc.ArrayDef) == 1 {
		if size, ok := arrayDimSize(dc.ArrayDef[0]); ok && size != len(cl.List) {
			diags = append(diags, Diagnostic{
				Code:     "timer-array-init-count",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"timer array %s: declared with %d element(s) but the initialiser supplies %d (ETSI 12)",
					quoted(name), size, len(cl.List)),
				Node: dc,
				Span: syntax.SpanOf(dc),
			})
		}
	}
	for _, e := range cl.List {
		if lit, ok := negativeNumericLiteral(e); ok {
			diags = append(diags, Diagnostic{
				Code:     "timer-negative-default",
				Severity: SeverityError,
				Message: "timer array " + quoted(name) +
					": default duration " + lit +
					" must be non-negative (ETSI 12)",
				Node: e,
				Span: syntax.SpanOf(e),
			})
		}
	}
	return diags
}

// isTimerValueDecl reports whether vd declares a timer. The
// parser stores `timer` either in `KindTok` (when a local
// declaration uses the kind-keyword form) or in `Type` as an
// identifier whose underlying token is TIMER (for the
// component-body shorthand `timer t := ...`).
func isTimerValueDecl(vd *syntax.ValueDecl) bool {
	if vd == nil {
		return false
	}
	if vd.KindTok != nil && vd.KindTok.Kind() == syntax.TIMER {
		return true
	}
	if id, ok := vd.Type.(*syntax.Ident); ok && id != nil && id.Tok != nil {
		if id.Tok.Kind() == syntax.TIMER {
			return true
		}
	}
	return false
}

// negativeNumericLiteral returns the printed form when expr is
// `-<number>` (UnaryExpr over a ValueLiteral that parses as a
// numeric token).
func negativeNumericLiteral(expr syntax.Expr) (string, bool) {
	ue, ok := expr.(*syntax.UnaryExpr)
	if !ok || ue == nil || ue.Op == nil {
		return "", false
	}
	if ue.Op.String() != "-" {
		return "", false
	}
	vl, ok := ue.X.(*syntax.ValueLiteral)
	if !ok || vl == nil || vl.Tok == nil {
		return "", false
	}
	s := vl.Tok.String()
	if !isNumericLiteralText(s) {
		return "", false
	}
	return "-" + s, true
}

// integerOnlyLiteral returns true when expr is a bare integer
// literal (no decimal point, no exponent) used as a timer
// default duration. Timer durations are floats in TTCN-3 and
// integers are not legal initialisers (ETSI 12).
func integerOnlyLiteral(expr syntax.Expr) bool {
	vl, ok := expr.(*syntax.ValueLiteral)
	if !ok || vl == nil || vl.Tok == nil {
		return false
	}
	s := vl.Tok.String()
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

func isNumericLiteralText(s string) bool {
	if s == "" {
		return false
	}
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' {
			continue
		}
		return false
	}
	return hasDigit
}

func quoted(s string) string {
	if s == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteByte('"')
	sb.WriteString(s)
	sb.WriteByte('"')
	return sb.String()
}
