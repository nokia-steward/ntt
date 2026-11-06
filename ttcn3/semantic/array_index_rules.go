// array_index_rules.go enforces ETSI ES 201 873-1 clause 6.2.7
// individual element access of arrays:
//
//   - the index value must be of integer kind (or record-of-integer
//     for nested access). A bare string / float / boolean literal
//     is rejected.
//
//   - the index value must be within the array's declared bounds
//     ([0..size-1]). Static literal indices >= size are rejected;
//     non-literal indices fall through.
//
// We only diagnose cases where both the array type and the literal
// index are statically known. Anything dynamic (function returns,
// non-literal index, unknown variable type) is silently skipped.
package semantic

import (
	"fmt"
	"strconv"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type arraySpec struct {
	// size is the total number of slots when the dims are all
	// integer literals or static ranges, 0 when unknown.
	size int64
	// lo / hi are the inclusive index bounds for the *outermost*
	// dimension when expressed as `[lo..hi]`. For plain `[N]`
	// dims they are 0 / N-1. When unknown (e.g. variable-size
	// record-of), boundsKnown is false.
	lo, hi      int64
	boundsKnown bool
}

func (a *Analyzer) checkArrayIndexRules(mod *syntax.Module) []Diagnostic {
	arrayTypes := collectArrayTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		localArrays := collectLocalArrayVars(fn.Body, arrayTypes)
		if len(localArrays) == 0 {
			return false
		}
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			ie, ok := sn.(*syntax.IndexExpr)
			if !ok || ie == nil {
				return true
			}
			varName := identName(ie.X)
			spec, ok := localArrays[varName]
			if !ok {
				return true
			}
			diags = append(diags, checkArrayIndexExpr(ie, varName, spec)...)
			return true
		})
		return false
	})
	return diags
}

func checkArrayIndexExpr(ie *syntax.IndexExpr, varName string, spec arraySpec) []Diagnostic {
	var diags []Diagnostic
	if ie.Index == nil {
		return diags
	}
	lit, ok := ie.Index.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return diags
	}
	switch lit.Tok.Kind() {
	case syntax.STRING:
		diags = append(diags, Diagnostic{
			Code:     "array-index-non-integer",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"array %q indexed with a string literal %q (must be integer)",
				varName, lit.Tok.String()),
			Node: ie,
			Span: syntax.SpanOf(ie),
		})
	case syntax.FLOAT:
		diags = append(diags, Diagnostic{
			Code:     "array-index-non-integer",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"array %q indexed with a float literal %s (must be integer)",
				varName, lit.Tok.String()),
			Node: ie,
			Span: syntax.SpanOf(ie),
		})
	case syntax.BSTRING:
		diags = append(diags, Diagnostic{
			Code:     "array-index-non-integer",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"array %q indexed with a binary-string literal %s (must be integer)",
				varName, lit.Tok.String()),
			Node: ie,
			Span: syntax.SpanOf(ie),
		})
	case syntax.INT:
		v, err := strconv.ParseInt(lit.Tok.String(), 10, 64)
		if err != nil {
			return diags
		}
		if spec.boundsKnown {
			if v < spec.lo || v > spec.hi {
				diags = append(diags, Diagnostic{
					Code:     "array-index-out-of-bounds",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"array %q index %d out of bounds (valid indices %d..%d)",
						varName, v, spec.lo, spec.hi),
					Node: ie,
					Span: syntax.SpanOf(ie),
				})
			}
		} else if spec.size > 0 {
			if v < 0 || v >= spec.size {
				diags = append(diags, Diagnostic{
					Code:     "array-index-out-of-bounds",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"array %q index %d out of bounds (size %d, valid indices 0..%d)",
						varName, v, spec.size, spec.size-1),
					Node: ie,
					Span: syntax.SpanOf(ie),
				})
			}
		}
	}
	return diags
}

// collectArrayTypes walks SubTypeDecl bodies looking for
// `type T BaseT[N];` shapes and records the per-type slot count.
//
// Also records the type names of record-of / set-of declarations.
// For length-constrained `type record length(lo..hi) of T Name;`
// shapes we capture the upper bound so the index check can
// reject `v[hi]` and beyond on the LHS of an assignment. Plain
// (unconstrained) record-of / set-of subtypes still get an entry
// with size==0 so the non-integer index check fires on them.
func collectArrayTypes(mod *syntax.Module) map[string]arraySpec {
	out := map[string]arraySpec{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch sd := n.(type) {
		case *syntax.SubTypeDecl:
			if sd == nil || sd.Field == nil || sd.Field.Name == nil {
				return true
			}
			if len(sd.Field.ArrayDef) > 0 {
				out[sd.Field.Name.String()] = arraySpecFromDims(sd.Field.ArrayDef)
				return true
			}
			ls, ok := sd.Field.Type.(*syntax.ListSpec)
			if !ok || ls == nil || ls.KindTok == nil {
				return true
			}
			switch ls.KindTok.Kind() {
			case syntax.RECORD, syntax.SET:
			default:
				return true
			}
			spec := arraySpec{size: 0}
			le := ls.Length
			if le == nil {
				le = sd.Field.LengthConstraint
			}
			if le != nil {
				if hi, ok := lengthExprMax(le); ok && hi > 0 {
					spec = arraySpec{
						lo:          0,
						hi:          int64(hi - 1),
						boundsKnown: true,
						size:        int64(hi),
					}
				}
			}
			out[sd.Field.Name.String()] = spec
		}
		return true
	})
	return out
}

// lengthExprMax extracts the upper bound (inclusive) of a length
// constraint. Returns (0, false) when the bound is non-literal or
// unbounded (`length(N..infinity)`).
func lengthExprMax(le *syntax.LengthExpr) (int, bool) {
	if le == nil || le.Size == nil || len(le.Size.List) == 0 {
		return 0, false
	}
	expr := le.Size.List[0]
	if be, ok := expr.(*syntax.BinaryExpr); ok && be != nil &&
		be.Op != nil && be.Op.Kind() == syntax.RANGE {
		hi, ok := intLiteral64(be.Y)
		if !ok || hi <= 0 {
			return 0, false
		}
		return int(hi), true
	}
	if n, ok := intLiteral64(expr); ok && n > 0 {
		return int(n), true
	}
	return 0, false
}

// arraySpecFromDims walks every dimension expression in a
// FormalArrayDef and produces an arraySpec. The outermost
// dimension's [lo..hi] (or [N] => [0..N-1]) is captured for the
// per-index bound check. Inner dims fold into `size` so the
// total-slots fallback still works.
func arraySpecFromDims(arrayDefs []*syntax.ParenExpr) arraySpec {
	spec := arraySpec{}
	total := int64(1)
	for i, pe := range arrayDefs {
		if pe == nil {
			continue
		}
		for j, dim := range pe.List {
			if dim == nil {
				continue
			}
			lo, hi, ok := dimRange(dim)
			if !ok {
				return arraySpec{}
			}
			if i == 0 && j == 0 {
				spec.lo = lo
				spec.hi = hi
				spec.boundsKnown = true
			}
			width := hi - lo + 1
			if width <= 0 {
				return arraySpec{}
			}
			total *= width
		}
	}
	if total == 1 && !spec.boundsKnown {
		return arraySpec{}
	}
	spec.size = total
	return spec
}

// dimRange interprets a dimension expression. A bare INT literal
// `N` is treated as `[0..N-1]`. A `lo..hi` BinaryExpr with two
// integer literals returns the explicit bounds.
func dimRange(dim syntax.Expr) (int64, int64, bool) {
	if lit, ok := dim.(*syntax.ValueLiteral); ok && lit != nil &&
		lit.Tok != nil && lit.Tok.Kind() == syntax.INT {
		n, err := strconv.ParseInt(lit.Tok.String(), 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		return 0, n - 1, true
	}
	be, ok := dim.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.RANGE {
		return 0, 0, false
	}
	lo, okLo := intLiteral64(be.X)
	hi, okHi := intLiteral64(be.Y)
	if !okLo || !okHi {
		return 0, 0, false
	}
	return lo, hi, true
}

func intLiteral64(e syntax.Expr) (int64, bool) {
	if lit, ok := e.(*syntax.ValueLiteral); ok && lit != nil &&
		lit.Tok != nil && lit.Tok.Kind() == syntax.INT {
		v, err := strconv.ParseInt(lit.Tok.String(), 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	return 0, false
}

func collectLocalArrayVars(body syntax.Node, arrayTypes map[string]arraySpec) map[string]arraySpec {
	out := map[string]arraySpec{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := syntax.Name(vd.Type)
		spec, isArrayType := arrayTypes[typeName]
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			declSpec := spec
			if !isArrayType && len(dec.ArrayDef) > 0 {
				// inline `var T x[N]` or `var T x[lo..hi]`
				declSpec = arraySpecFromDims(dec.ArrayDef)
			} else if !isArrayType {
				continue
			}
			out[dec.Name.String()] = declSpec
		}
		return true
	})
	return out
}
