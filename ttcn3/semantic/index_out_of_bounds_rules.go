// index_out_of_bounds_rules.go enforces ETSI ES 201 873-1 clause
// 6.2.3.2 by flagging `v[N]` indexed reads on a record-of /
// set-of variable initialised with a fixed-length composite
// literal when N is provably outside that length.
//
// The check stays narrow: it only fires when the variable is
// declared with a CompositeLiteral initializer and is never
// reassigned in the enclosing function body. That keeps us off
// dynamic cases that the type checker isn't ready for yet.
//
// Catches NegSem_060203_..._013 and _014.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkIndexOutOfBoundsRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		sizes := collectCompositeLiteralSizes(fn.Body)
		if len(sizes) == 0 {
			continue
		}
		diags = append(diags, indexOutOfBoundsDiags(fn.Body, sizes)...)
	}
	return diags
}

// collectCompositeLiteralSizes returns the size of each variable
// initialised by a CompositeLiteral whose elements are all
// "specific" (no `*` wildcards). Variables that are reassigned
// later are dropped because the new RHS may have a different
// size.
func collectCompositeLiteralSizes(body *syntax.BlockStmt) map[string]int {
	sizes := map[string]int{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			cl, ok := dec.Value.(*syntax.CompositeLiteral)
			if !ok || cl == nil {
				continue
			}
			if hasNamedOrIndexedElement(cl) {
				// Indexed / named elements make the
				// effective size dynamic: `{ [5] := 1 }`
				// is length 6, but the literal carries
				// a single element. Punt rather than
				// flag a phantom out-of-bounds.
				continue
			}
			specifics, anyOrNone := compositeElementCounts(cl)
			if anyOrNone > 0 {
				continue
			}
			sizes[dec.Name.String()] = specifics
		}
		return true
	})
	if len(sizes) == 0 {
		return sizes
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		// Plain reassignment `v := ...` drops the candidate.
		if id, ok := be.X.(*syntax.Ident); ok {
			delete(sizes, id.String())
			return true
		}
		// Indexed writes (`v[i] := ...`) grow record-of
		// values so we lose static knowledge of the size.
		if idx, ok := be.X.(*syntax.IndexExpr); ok && idx != nil {
			if id, ok := idx.X.(*syntax.Ident); ok {
				delete(sizes, id.String())
			}
		}
		return true
	})
	return sizes
}

// hasNamedOrIndexedElement returns true when any element of cl
// uses named-field (`f := v`) or indexed (`[i] := v`) notation.
// Such literals have a dynamic effective length that we can't
// summarise with a single integer, so the bound check has to
// bail out.
func hasNamedOrIndexedElement(cl *syntax.CompositeLiteral) bool {
	for _, e := range cl.List {
		if _, _, ok := assignmentNotationName(e); ok {
			return true
		}
		if _, _, ok := indexAssignmentValue(e); ok {
			return true
		}
	}
	return false
}

func indexOutOfBoundsDiags(body *syntax.BlockStmt, sizes map[string]int) []Diagnostic {
	// Pre-collect every IndexExpr that's used as the LHS of
	// an `:=` so the second pass can skip it. Indexed writes
	// are allowed to grow a record-of and are caught by the
	// run-time bounds check rather than this static rule.
	writeSide := map[syntax.Node]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		if idx, ok := be.X.(*syntax.IndexExpr); ok && idx != nil {
			writeSide[idx] = true
		}
		return true
	})

	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		idx, ok := n.(*syntax.IndexExpr)
		if !ok || idx == nil || idx.X == nil || idx.Index == nil {
			return true
		}
		if writeSide[idx] {
			return true
		}
		id, ok := idx.X.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		size, known := sizes[id.String()]
		if !known {
			return true
		}
		n64, ok := signedIntLiteral(idx.Index)
		if !ok {
			return true
		}
		if n64 < 0 || n64 >= int64(size) {
			diags = append(diags, Diagnostic{
				Code:     "index-out-of-bounds",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"index %d is out of bounds for %q (length %d) (ETSI 6.2.3.2)",
					n64, id.String(), size),
				Node: idx,
				Span: syntax.SpanOf(idx),
			})
		}
		return true
	})
	return diags
}
