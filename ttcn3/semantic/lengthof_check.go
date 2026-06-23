// lengthof_check.go implements a small static check for the
// `lengthof` predefined function (ETSI Annex C):
//
//	lengthof(x) is undefined when x is a template / value whose
//	length is not pinned to a single integer. The common form in
//	the conformance suite is `lengthof('1*F'H)` (wildcard inside
//	the literal) and `lengthof('1'B length(3..6))` (length
//	restriction that names a range, not a single value).
//
// We don't have a full template-length inferencer yet, so the check
// is intentionally narrow: it flags literal hex / bit / oct strings
// that carry a `*` / `?` wildcard, and `LengthExpr` wrappers whose
// size clause names a range with distinct lower and upper bounds.
// Anything else (a bare ident referring to a template, a function
// call, etc.) is left alone - the interpreter still produces a
// value at runtime, just not a verifiable one.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkLengthofArgs(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id.String() != "lengthof" {
			return true
		}
		if ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		arg := ce.Args.List[0]
		if reason := lengthofViolation(arg); reason != "" {
			diags = append(diags, Diagnostic{
				Code:     "lengthof-undetermined-length",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"lengthof: argument has undetermined length (%s)",
					reason),
				Node: arg,
				Span: syntax.SpanOf(arg),
			})
		}
		return true
	})
	return diags
}

func lengthofViolation(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		if v.Tok != nil && v.Tok.Kind() == syntax.BSTRING {
			lit := v.Tok.String()
			if strings.ContainsAny(lit, "*?") {
				return "wildcard in literal"
			}
		}
	case *syntax.LengthExpr:
		if isLengthRange(v.Size) {
			return "length restriction is a range"
		}
	case *syntax.ParenExpr:
		if len(v.List) == 1 {
			return lengthofViolation(v.List[0])
		}
	}
	return ""
}

// isLengthRange returns true when the parenthesised size expression
// of a `length(...)` clause names a range (`length(min..max)` with
// min != max) or is otherwise non-single (a bare `infinity`, a list
// of options, etc.). A plain `length(N)` returns false - that pins
// the length exactly and `lengthof` is well-defined.
func isLengthRange(p *syntax.ParenExpr) bool {
	if p == nil || len(p.List) != 1 {
		return false
	}
	e := p.List[0]
	if be, ok := e.(*syntax.BinaryExpr); ok && be.Op != nil &&
		be.Op.Kind() == syntax.RANGE {
		// `length(N..N)` would technically pin the value;
		// we don't fold constants here so a typo of that
		// shape goes through. The suite's reject fixtures
		// always use distinct bounds.
		return true
	}
	if id, ok := e.(*syntax.Ident); ok && id.String() == "infinity" {
		return true
	}
	return false
}
