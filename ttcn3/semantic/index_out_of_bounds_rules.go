// index_out_of_bounds_rules.go enforces ETSI ES 201 873-1 clause
// 6.2.3.2 by flagging `v[N]` indexed reads on a record-of /
// set-of variable initialised with a fixed-length composite
// literal when N is provably outside that length.
//
// The check stays narrow: it only fires when the variable is
// declared with a CompositeLiteral initializer and is never
// reassigned (nor passed by reference to an out / inout
// parameter, 5.4.2) in the enclosing function body. That keeps
// us off dynamic cases that the type checker isn't ready for yet.
//
// Not every indexed reference is a read that must resolve:
//
//   - The ExtendedFieldReference of a presence checking operator
//     yields false instead of an error (7.1.8.0); an index used
//     inside one of its index expressions is still evaluated.
//   - An actual out / inout parameter follows the left hand side
//     rules, so it expands the value (5.4.2).
//
// Arrays declared with a range dimension (`[4 .. 6]`) index from
// their lower bound (6.2.7), and template literals are counted in
// their transformed form (15.6.3): a permutation shelters one
// index per element and `* length(n)` stands for n elements.
//
// Catches NegSem_060203_..._013 and _014.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// literalBounds is what the rule knows about a variable initialised
// by a composite literal: its n elements are indexed from lo.
type literalBounds struct {
	lo, n int64
}

// presenceCheckOps are the presence checking operators of 7.1.8.0.
var presenceCheckOps = map[string]bool{
	"isbound": true, "ispresent": true, "ischosen": true, "isvalue": true,
}

func (a *Analyzer) checkIndexOutOfBoundsRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	arrayDims := collectArrayTypeDims(mod)
	byRef := collectByRefParams(mod)
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		sizes := collectCompositeLiteralSizes(fn.Body, arrayDims, byRef)
		if len(sizes) == 0 {
			continue
		}
		diags = append(diags, indexOutOfBoundsDiags(fn.Body, sizes, byRef)...)
	}
	return diags
}

// collectCompositeLiteralSizes returns the bounds of each variable
// initialised by a CompositeLiteral whose transformed length is
// known (no `*` wildcards). Variables that are reassigned or passed
// by reference later are dropped because the new value may have a
// different size.
func collectCompositeLiteralSizes(body *syntax.BlockStmt, arrayDims map[string][]*syntax.ParenExpr, byRef byRefParams) map[string]literalBounds {
	sizes := map[string]literalBounds{}
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
			size, ok := transformedLength(cl)
			if !ok {
				continue
			}
			dims := dec.ArrayDef
			if len(dims) == 0 {
				dims = arrayDims[identName(vd.Type)]
			}
			lo, ok := arrayLowerBound(dims)
			if !ok {
				continue
			}
			sizes[dec.Name.String()] = literalBounds{lo: lo, n: int64(size)}
		}
		return true
	})
	if len(sizes) == 0 {
		return sizes
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		// Passing `v` or `v[i]` to an out / inout parameter
		// may resize v just like an assignment.
		if ce, ok := n.(*syntax.CallExpr); ok && ce != nil {
			for _, arg := range byRef.args(ce) {
				if name := rootIdentName(arg); name != "" {
					delete(sizes, name)
				}
			}
			return true
		}
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

// transformedLength counts the elements of a record of / set of
// literal in the transformed form of 15.6.3: `* length(n)` stands
// for n AnyElements and a permutation for one element per member.
// A bare `*`, a ranged length, or a `*` inside a permutation leaves
// the length (and the sheltered indexes) open.
func transformedLength(cl *syntax.CompositeLiteral) (int, bool) {
	n := 0
	for _, e := range cl.List {
		switch v := e.(type) {
		case *syntax.LengthExpr:
			if v != nil && isAnyValueOrNone(v.X) {
				lr, _, _, ok := parseLengthRange(v)
				if !ok || lr.exclusive || !lr.hasUpper || lr.min != lr.max {
					return 0, false
				}
				n += lr.min
				continue
			}
		case *syntax.CallExpr:
			if v != nil && identName(v.Fun) == "permutation" && v.Args != nil {
				for _, m := range v.Args.List {
					if _, ok := m.(*syntax.LengthExpr); ok || isAnyValueOrNone(m) {
						return 0, false
					}
				}
				n += len(v.Args.List)
				continue
			}
		}
		if isAnyValueOrNone(e) {
			return 0, false
		}
		n++
	}
	return n, true
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

// collectArrayTypeDims maps each array type of the module
// (`type integer A[2 .. 5];`) to its dimensions.
func collectArrayTypeDims(mod *syntax.Module) map[string][]*syntax.ParenExpr {
	out := map[string][]*syntax.ParenExpr{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || sd.Field == nil || sd.Field.Name == nil || len(sd.Field.ArrayDef) == 0 {
			continue
		}
		out[sd.Field.Name.String()] = sd.Field.ArrayDef
	}
	return out
}

// arrayLowerBound returns the first index of the outermost array
// dimension: zero for non-arrays and `[N]`, lo for `[lo .. hi]`.
// It fails when a range bound is not an integer literal.
func arrayLowerBound(dims []*syntax.ParenExpr) (int64, bool) {
	if len(dims) == 0 || dims[0] == nil || len(dims[0].List) == 0 {
		return 0, true
	}
	be, ok := dims[0].List[0].(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.RANGE {
		return 0, true
	}
	return signedIntLiteral(be.X)
}

// markPathIndexes adds the index notations along the reference e to
// set, without descending into the index expressions themselves.
func markPathIndexes(e syntax.Expr, set map[syntax.Node]bool) {
	for {
		switch v := e.(type) {
		case *syntax.IndexExpr:
			set[v] = true
			e = v.X
		case *syntax.SelectorExpr:
			e = v.X
		default:
			return
		}
	}
}

func indexOutOfBoundsDiags(body *syntax.BlockStmt, sizes map[string]literalBounds, byRef byRefParams) []Diagnostic {
	// Pre-collect every IndexExpr that's used as the LHS of
	// an `:=` so the second pass can skip it. Indexed writes
	// are allowed to grow a record-of and are caught by the
	// run-time bounds check rather than this static rule. The
	// operand of a presence checking operator and actual
	// out / inout parameters are skipped likewise.
	writeSide := map[syntax.Node]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if ce, ok := n.(*syntax.CallExpr); ok && ce != nil {
			if presenceCheckOps[identName(ce.Fun)] && ce.Args != nil && len(ce.Args.List) == 1 {
				markPathIndexes(ce.Args.List[0], writeSide)
			}
			for _, arg := range byRef.args(ce) {
				markPathIndexes(arg, writeSide)
			}
			return true
		}
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
		b, known := sizes[id.String()]
		if !known {
			return true
		}
		n64, ok := signedIntLiteral(idx.Index)
		if !ok {
			return true
		}
		if n64 < b.lo || n64 >= b.lo+b.n {
			diags = append(diags, Diagnostic{
				Code:     "index-out-of-bounds",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"index %d is out of bounds for %q (length %d) (ETSI 6.2.3.2)",
					n64, id.String(), b.n),
				Node: idx,
				Span: syntax.SpanOf(idx),
			})
		}
		return true
	})
	return diags
}
