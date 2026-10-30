// length_subtyping_rules.go enforces ETSI ES 201 873-1 clause
// 6.2.13.1 "Length subtyping" on record-of and set-of types:
//
//   - length(...) bounds must use *inclusive* boundaries; the
//     parser surfaces an exclusive `!N` boundary as
//     UnaryExpr{Op: !, X: N}, so any such operand is rejected.
//   - A subtype's length(...) range must lie within its parent's
//     effective length range. We walk the parent typedef chain
//     gathering the most restrictive lo/hi seen so far and emit a
//     diagnostic if the current declaration's range escapes it.
//
// The rule is intentionally minimal: it only fires when both the
// parent chain and the current length(...) bounds resolve to
// integer literals. Symbolic ranges are silently skipped.
package semantic

import (
	"fmt"
	"math"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type lengthRange struct {
	min       int
	max       int
	hasUpper  bool
	exclusive bool
}

func (a *Analyzer) checkLengthSubtypingRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	parents := map[string]string{}
	ranges := map[string]lengthRange{}

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
			continue
		}
		name := st.Field.Name.String()
		// Direct `type record length(...) of T <name>` shape.
		if ls, ok := st.Field.Type.(*syntax.ListSpec); ok && ls != nil && ls.Length != nil {
			lr, exclSpan, badNode, ok := parseLengthRange(ls.Length)
			if ok {
				ranges[name] = lr
				if lr.exclusive {
					diags = append(diags, Diagnostic{
						Code:     "length-subtype-exclusive-bound",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"length(...) on subtype %q uses an exclusive boundary; only inclusive boundaries are allowed (ETSI 6.2.13.1)",
							name),
						Node: badNode,
						Span: exclSpan,
					})
				}
			}
		}
		// `type Parent <name> length(...)` shape.
		if st.Field.LengthConstraint != nil {
			lr, exclSpan, badNode, ok := parseLengthRange(st.Field.LengthConstraint)
			if ok {
				ranges[name] = lr
				if lr.exclusive {
					diags = append(diags, Diagnostic{
						Code:     "length-subtype-exclusive-bound",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"length(...) on subtype %q uses an exclusive boundary; only inclusive boundaries are allowed (ETSI 6.2.13.1)",
							name),
						Node: badNode,
						Span: exclSpan,
					})
				}
			}
		}
		if ref, ok := st.Field.Type.(*syntax.RefSpec); ok && ref != nil {
			if id, ok := ref.X.(*syntax.Ident); ok && id != nil {
				parents[name] = id.String()
			}
		}
	}

	for name, lr := range ranges {
		if lr.exclusive {
			continue
		}
		hi := math.MaxInt
		lo := 0
		hasHi := false
		seen := map[string]bool{name: true}
		for p := parents[name]; p != ""; p = parents[p] {
			if seen[p] {
				break
			}
			seen[p] = true
			pr, ok := ranges[p]
			if !ok || pr.exclusive {
				continue
			}
			if pr.min > lo {
				lo = pr.min
			}
			if pr.hasUpper && pr.max < hi {
				hi = pr.max
				hasHi = true
			}
		}
		if lr.min < lo || (hasHi && lr.hasUpper && lr.max > hi) ||
			(hasHi && !lr.hasUpper) {
			diags = append(diags, Diagnostic{
				Code:     "length-subtype-out-of-range",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"length(...) on subtype %q falls outside the parent's effective range [%s] (ETSI 6.2.13.1)",
					name, describeRange(lo, hi, hasHi)),
				Node: nil,
				Span: syntax.Span{},
			})
		}
	}
	return diags
}

func describeRange(lo, hi int, hasHi bool) string {
	if hasHi {
		return fmt.Sprintf("%d..%d", lo, hi)
	}
	return fmt.Sprintf("%d..infinity", lo)
}

// parseLengthRange flattens a LengthExpr into a lengthRange. The
// exclusive flag is set when any operand uses the `!N` (UnaryExpr{!})
// shape, which TTCN-3 forbids for length subtyping. badNode / span
// point at the offending exclusive boundary, or the LengthExpr
// itself otherwise.
func parseLengthRange(le *syntax.LengthExpr) (lengthRange, syntax.Span, syntax.Node, bool) {
	if le == nil || le.Size == nil || len(le.Size.List) != 1 {
		return lengthRange{}, syntax.Span{}, nil, false
	}
	expr := le.Size.List[0]
	lr := lengthRange{}
	switch v := expr.(type) {
	case *syntax.BinaryExpr:
		if v.Op == nil || v.Op.String() != ".." {
			return lengthRange{}, syntax.Span{}, nil, false
		}
		lo, excludeLo, badLo, ok := parseLengthOperand(v.X)
		if !ok {
			return lengthRange{}, syntax.Span{}, nil, false
		}
		hi, excludeHi, badHi, hasHi := parseLengthOperand(v.Y)
		lr.min = lo
		if hasHi {
			lr.max = hi
			lr.hasUpper = true
		}
		if excludeLo {
			lr.exclusive = true
			return lr, syntax.SpanOf(badLo), badLo, true
		}
		if excludeHi {
			lr.exclusive = true
			return lr, syntax.SpanOf(badHi), badHi, true
		}
		return lr, syntax.SpanOf(le), le, true
	case *syntax.ValueLiteral:
		n, _, _, ok := parseLengthOperand(v)
		if !ok {
			return lengthRange{}, syntax.Span{}, nil, false
		}
		lr.min = n
		lr.max = n
		lr.hasUpper = true
		return lr, syntax.SpanOf(le), le, true
	}
	return lengthRange{}, syntax.Span{}, nil, false
}

// parseLengthOperand reads a single bound. Returns the value, the
// exclusive flag (true when the bound uses `!N`), the operand node
// (for diagnostics), and whether the bound could be parsed.
func parseLengthOperand(e syntax.Expr) (int, bool, syntax.Node, bool) {
	switch v := e.(type) {
	case *syntax.UnaryExpr:
		if v.Op == nil || v.Op.String() != "!" {
			return 0, false, nil, false
		}
		n, _, _, ok := parseLengthOperand(v.X)
		if !ok {
			return 0, true, v, true
		}
		return n, true, v, true
	case *syntax.ValueLiteral:
		if v.Tok == nil || v.Tok.Kind() != syntax.INT {
			return 0, false, nil, false
		}
		var n int
		if _, err := fmt.Sscanf(v.Tok.String(), "%d", &n); err != nil {
			return 0, false, nil, false
		}
		return n, false, v, true
	}
	return 0, false, nil, false
}
