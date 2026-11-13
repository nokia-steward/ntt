// subset_length_rules.go enforces ETSI ES 201 873-1 clause
// B.1.2.6 restriction f: when a \`subset(...)\` template
// matcher carries a \`length\` attribute, the upper bound
// of the length must not exceed the number of elements in
// the subset. The same applies symmetrically to
// \`superset(...)\` matchers where the lower bound of the
// length must not be less than the number of elements
// supplied (B.1.2.7).
//
// We only flag the cleanest, fully literal shape:
//   - the matcher is a call \`subset(...)\` / \`superset(...)\`
//     with at least one positional element argument,
//   - the matcher is followed by a \`length\` clause whose
//     bound expression is an integer literal or
//     \`X .. Y\` literal range.
package semantic

import (
	"fmt"
	"strconv"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSubsetLengthRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		le, ok := n.(*syntax.LengthExpr)
		if !ok || le == nil || le.X == nil || le.Size == nil ||
			len(le.Size.List) == 0 {
			return true
		}
		ce, ok := le.X.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil {
			return true
		}
		fnID, ok := ce.Fun.(*syntax.Ident)
		if !ok || fnID == nil {
			return true
		}
		op := fnID.String()
		if op != "subset" && op != "superset" {
			return true
		}
		count := len(ce.Args.List)
		if count == 0 {
			return true
		}
		lo, hi, ok := literalLengthBounds(le.Size.List[0])
		if !ok {
			return true
		}
		if op == "subset" && hi > 0 && hi > count {
			diags = append(diags, Diagnostic{
				Code:     "subset-length-too-large",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`subset` has %d element(s); the `length` upper bound %d cannot exceed that count (ETSI B.1.2.6 f)",
					count, hi),
				Node: le,
				Span: syntax.SpanOf(le),
			})
		}
		if op == "superset" && lo > 0 && lo < count {
			diags = append(diags, Diagnostic{
				Code:     "superset-length-too-small",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`superset` has %d element(s); the `length` lower bound %d cannot be less than that count (ETSI B.1.2.7 f)",
					count, lo),
				Node: le,
				Span: syntax.SpanOf(le),
			})
		}
		return true
	})
	return diags
}

func literalLengthBounds(expr syntax.Expr) (int, int, bool) {
	if vl, ok := expr.(*syntax.ValueLiteral); ok && vl != nil && vl.Tok != nil {
		if n, err := strconv.Atoi(vl.Tok.String()); err == nil {
			return n, n, true
		}
		return 0, 0, false
	}
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.RANGE {
		return 0, 0, false
	}
	loVL, ok := be.X.(*syntax.ValueLiteral)
	if !ok || loVL == nil || loVL.Tok == nil {
		return 0, 0, false
	}
	lo, err := strconv.Atoi(loVL.Tok.String())
	if err != nil {
		return 0, 0, false
	}
	if id, ok := be.Y.(*syntax.Ident); ok && id != nil && id.String() == "infinity" {
		return lo, -1, true
	}
	hiVL, ok := be.Y.(*syntax.ValueLiteral)
	if !ok || hiVL == nil || hiVL.Tok == nil {
		return lo, 0, false
	}
	hi, err := strconv.Atoi(hiVL.Tok.String())
	if err != nil {
		return lo, 0, false
	}
	return lo, hi, true
}
