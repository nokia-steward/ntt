// composite_literal_rules.go enforces ETSI ES 201 873-1 clause
// 6.2 well-formedness restrictions on composite (record / set /
// union) literal forms:
//
//   - mixed value-list and assignment notation inside the same
//     immediate context is forbidden (NegSyn_0602_TopLevel_003);
//   - duplicate field names in the assignment notation are
//     forbidden (NegSyn_0602_TopLevel_004 / _006).
//
// The check operates on the AST shape only, so it never has to
// model the target type. Anything that doesn't match the two
// canonical shapes is silently ignored.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkCompositeLiteralRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		cl, ok := n.(*syntax.CompositeLiteral)
		if !ok || cl == nil || len(cl.List) == 0 {
			return true
		}
		var (
			seenAssign  bool
			seen        = map[string]syntax.Node{}
			seenIndices = map[int64]syntax.Node{}
		)
		for _, elt := range cl.List {
			if elt == nil {
				continue
			}
			if name, anchor, ok := assignmentNotationName(elt); ok {
				seenAssign = true
				if _, dup := seen[name]; dup {
					diags = append(diags, Diagnostic{
						Code:     "composite-literal-duplicate-field",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"field %q appears more than once in the same composite literal (ETSI 6.2)",
							name),
						Node: anchor,
						Span: syntax.SpanOf(anchor),
					})
					continue
				}
				seen[name] = anchor
				continue
			}
			if idx, anchor, ok := indexAssignmentValue(elt); ok {
				if _, dup := seenIndices[idx]; dup {
					diags = append(diags, Diagnostic{
						Code:     "composite-literal-duplicate-index",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"index [%d] appears more than once in the same composite literal (ETSI 6.2)",
							idx),
						Node: anchor,
						Span: syntax.SpanOf(anchor),
					})
					continue
				}
				seenIndices[idx] = anchor
				continue
			}
			if seenAssign {
				// Positional element after a named one
				// is the only mixed-notation shape
				// ETSI 6.2 actually forbids; the
				// `value-list first, named-list
				// after` order is legal and is
				// exercised by Sem_0602_TopLevel_22.
				diags = append(diags, Diagnostic{
					Code:     "composite-literal-positional-after-named",
					Severity: SeverityError,
					Message:  "positional element cannot follow a named field in the same composite literal (ETSI 6.2)",
					Node:     elt,
					Span:     syntax.SpanOf(elt),
				})
			}
		}
		return true
	})
	return diags
}

// assignmentNotationName returns the field name (and the node to
// blame for diagnostics) when expr is `<ident> := <value>`.
// Anything else returns ok=false and the caller treats the
// element as positional / value-list notation.
func assignmentNotationName(expr syntax.Expr) (string, syntax.Node, bool) {
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return "", nil, false
	}
	id, ok := be.X.(*syntax.Ident)
	if !ok || id == nil {
		return "", nil, false
	}
	return id.String(), id, true
}

// indexAssignmentValue matches `[N] := value` shapes used by
// record-of / set-of / array literals. When the index is a
// literal integer (or unary minus literal) we return its value
// and the anchor node for diagnostics. Anything more dynamic
// (`[v_i] := ...`) returns ok=false.
func indexAssignmentValue(expr syntax.Expr) (int64, syntax.Node, bool) {
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return 0, nil, false
	}
	idx, ok := be.X.(*syntax.IndexExpr)
	if !ok || idx == nil || idx.Index == nil {
		return 0, nil, false
	}
	v, ok := signedIntLiteral(idx.Index)
	if !ok {
		return 0, nil, false
	}
	return v, idx, true
}
