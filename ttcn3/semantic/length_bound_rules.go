// length_bound_rules.go enforces ETSI ES 201 873-1 clause 6.1.2.4
// restrictions on the integer bounds of a `length(...)` constraint:
//
//   - bounds must be non-negative (NegSyn_06010204_StringLenghtRestrict_002);
//   - upper bound must be >= lower bound (NegSyn_06010204_StringLenghtRestrict_001).
//
// The check is purely syntactic and only fires when both bounds are
// integer literals (possibly prefixed by a unary `-`). Anything
// involving identifiers / expressions silently passes.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkLengthBoundRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		le, ok := n.(*syntax.LengthExpr)
		if !ok || le.Size == nil || len(le.Size.List) == 0 {
			return true
		}
		expr := le.Size.List[0]
		var loVal, hiVal int64
		hasLo, hasHi := false, false
		hiInf := false
		if !isLengthRangeBinary(expr) {
			if v, ok := signedIntLiteral(expr); ok {
				if v < 0 {
					diags = append(diags, lengthBoundDiag(expr,
						"length-fixed-negative",
						fmt.Sprintf("length(%d) is negative", v)))
				}
				loVal, hiVal = v, v
				hasLo, hasHi = true, true
			}
		} else {
			be := expr.(*syntax.BinaryExpr)
			loVal, hasLo = signedIntLiteral(be.X)
			if isInfinityExpr(be.Y) {
				hiInf = true
			} else {
				hiVal, hasHi = signedIntLiteral(be.Y)
			}
			if hasLo && loVal < 0 {
				diags = append(diags, lengthBoundDiag(be.X,
					"length-bound-negative",
					fmt.Sprintf("length lower bound %d is negative", loVal)))
			}
			if hasHi && hiVal < 0 {
				diags = append(diags, lengthBoundDiag(be.Y,
					"length-bound-negative",
					fmt.Sprintf("length upper bound %d is negative", hiVal)))
			}
			if hasLo && hasHi && hiVal < loVal {
				diags = append(diags, lengthBoundDiag(be,
					"length-bound-upper-below-lower",
					fmt.Sprintf("length upper bound %d is less than lower bound %d",
						hiVal, loVal)))
			}
		}
		// Inline literal length check: `'10'B length(N)` must
		// have N matching the literal's intrinsic length. This
		// covers NegSem_160102_predefined_functions_007.
		// Wildcard literals (`*`, `?`) are skipped because the
		// match length is dynamic.
		if le.X != nil && hasLo {
			if lit, ok := le.X.(*syntax.ValueLiteral); ok &&
				!literalHasWildcard(lit) {
				if litLen, _, ok := stringLiteralLength(lit); ok {
					litLen64 := int64(litLen)
					if litLen64 < loVal || (!hiInf && hasHi && litLen64 > hiVal) {
						diags = append(diags, lengthBoundDiag(le,
							"length-literal-out-of-range",
							fmt.Sprintf(
								"string literal of length %d is outside the declared length range",
								litLen)))
					}
				}
			}
			// Composite-literal length check: `{a, b, c, *}
			// length(1..2)` is a contradiction because the
			// template needs at least 3 elements but the
			// upper bound caps it at 2.
			if cl, ok := le.X.(*syntax.CompositeLiteral); ok {
				specifics, _ := compositeElementCounts(cl)
				minLen := int64(specifics)
				if hasHi && !hiInf && minLen > hiVal {
					diags = append(diags, lengthBoundDiag(le,
						"length-template-min-exceeds-bound",
						fmt.Sprintf(
							"composite template needs at least %d elements but the length range caps at %d",
							specifics, hiVal)))
				}
			}
		}
		return true
	})
	return diags
}

func isLengthRangeBinary(e syntax.Expr) bool {
	be, ok := e.(*syntax.BinaryExpr)
	if !ok || be.Op == nil {
		return false
	}
	return be.Op.String() == ".."
}

// compositeElementCounts returns (specificCount, anyOrNoneCount)
// for a CompositeLiteral, where:
//
//   - specificCount counts elements that contribute a fixed
//     minimum to the matched length (bare values, `?`,
//     permutation(...) - each `?` is exactly one slot);
//   - anyOrNoneCount counts `*` / `AnyValueOrNone` wildcards,
//     which match zero or more elements and therefore make the
//     overall lower bound unverifiable.
//
// We don't bother distinguishing finer template forms because the
// rule only uses the totals to compare against the declared
// length range.
func compositeElementCounts(cl *syntax.CompositeLiteral) (int, int) {
	if cl == nil {
		return 0, 0
	}
	var specifics, anyOrNone int
	for _, elt := range cl.List {
		if isAnyValueOrNone(elt) {
			anyOrNone++
			continue
		}
		specifics++
	}
	return specifics, anyOrNone
}

// isAnyValueOrNone matches `*` in a composite literal. We accept
// both the standalone literal form and the `*` keyword that the
// parser may surface as a ValueLiteral with a MUL token.
func isAnyValueOrNone(e syntax.Expr) bool {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return false
	}
	return lit.Tok.String() == "*"
}

// literalHasWildcard returns true when the literal token text
// contains a TTCN-3 matching mechanism (`?` or `*`) that makes
// the runtime match-length unbounded. We use a string scan
// rather than a token kind because the parser keeps the raw
// inner text on the bit-/hex-/octetstring literal token.
func literalHasWildcard(lit *syntax.ValueLiteral) bool {
	if lit == nil || lit.Tok == nil {
		return false
	}
	s := lit.Tok.String()
	for i := 0; i < len(s); i++ {
		if s[i] == '?' || s[i] == '*' {
			return true
		}
	}
	return false
}

func lengthBoundDiag(node syntax.Node, code, msg string) Diagnostic {
	return Diagnostic{
		Code:     code,
		Severity: SeverityError,
		Message:  msg + " (ETSI 6.1.2.4)",
		Node:     node,
		Span:     syntax.SpanOf(node),
	}
}
