// value_constraint.go implements ETSI ES 201 873-1 clause 6.1.2.3
// "Range" subtype validation for `type integer T (lo .. hi);` and
// `type float T (lo .. hi);` declarations. The check verifies that
// literal initialisers and direct assignments to a variable of the
// constrained type fall inside the declared range. We also flag the
// "value list" form (`type integer T (1, 2, 3);`) when the literal
// is not in the list.
//
// The implementation deliberately mirrors length_constraint.go so the
// rules can evolve together. The conservative bias is the same: we
// only look at literal RHS values; constants, identifiers, function
// returns, etc. fall through silently to keep the false-positive rate
// at zero.
package semantic

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// stripExcl peels an optional exclusive marker (`!`) off a range
// bound. TTCN-3's `..` range syntax can carry a leading `!` to mark
// either bound as exclusive: `(!10..!20)` is "everything strictly
// between 10 and 20". Returns the underlying expression and whether
// the marker was present. Nil input yields (nil, false) so the call
// sites can pass through bounds the parser handed back as nil for
// open ranges (`(infinity..10)`).
func stripExcl(e syntax.Expr) (syntax.Expr, bool) {
	if e == nil {
		return nil, false
	}
	if u, ok := e.(*syntax.UnaryExpr); ok && u.Op != nil && u.Op.Kind() == syntax.EXCL {
		return u.X, true
	}
	return e, false
}

func (a *Analyzer) checkValueConstraints(mod *syntax.Module) []Diagnostic {
	tab := collectValueConstrainedTypes(mod)
	var diags []Diagnostic
	// 6.1.2.3: validate the *form* of the range bounds for every
	// integer / float subtype regardless of whether we could fully
	// resolve the spec. This catches bound expressions that the
	// resolver silently dropped (e.g. `not_a_number`, `false..true`,
	// other non-numeric idents).
	diags = append(diags, checkRangeBoundForms(mod)...)
	if len(tab) > 0 {
		syntax.Inspect(mod, func(n syntax.Node) bool {
			if n == nil {
				return true
			}
			switch x := n.(type) {
			case *syntax.FuncDecl:
				if x.Body == nil {
					return false
				}
				vars := collectLocalValueConstrainedVars(x.Body, tab)
				diags = append(diags, checkValueInBody(x.Body, tab, vars)...)
				return false
			}
			if vd, ok := n.(*syntax.ValueDecl); ok {
				diags = append(diags, checkValueDeclInit(vd, tab)...)
			}
			return true
		})
	}
	return diags
}

// checkRangeBoundForms enforces ETSI 6.1.2.3 on the *shape* of
// range-constraint bounds: each lo / hi must be a numeric literal,
// infinity / -infinity, or a reference to a constant that resolves
// to one. Forbidden bound shapes include the boolean literals
// true / false (only integer / float / charstring / universal
// charstring are range-eligible) and the float NaN sentinel
// not_a_number (6.1.2.3 explicitly excludes it).
//
// String-base subtypes (charstring / universal charstring) reuse
// the same iteration but a different bound-shape validator,
// bogusStringRangeBound: infinity / -infinity are explicitly
// forbidden for string ranges by 6.1.2.3 because the string value
// domain has no defined infinite endpoint.
//
// The check runs independently of the literal-value resolver, so
// it fires even when the constraint contains shapes the resolver
// would have dropped silently.
func checkRangeBoundForms(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.ValueConstraint == nil {
			continue
		}
		numericBase := baseNumericFamily(st.Field.Type)
		stringBase := baseRangeableStringFamily(st.Field.Type)
		if numericBase == "" && stringBase == "" {
			continue
		}
		for _, item := range st.Field.ValueConstraint.List {
			be, ok := item.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.String() != ".." {
				continue
			}
			loExp, _ := stripExcl(be.X)
			hiExp, _ := stripExcl(be.Y)
			for _, b := range []syntax.Expr{loExp, hiExp} {
				var d *Diagnostic
				if numericBase != "" {
					d = bogusRangeBound(b, numericBase)
				} else if stringBase != "" {
					d = bogusStringRangeBound(b, stringBase)
				}
				if d != nil {
					d.Node = item
					d.Span = syntax.SpanOf(item)
					diags = append(diags, *d)
				}
			}
		}
	}
	return diags
}

// baseRangeableStringFamily widens the existing
// length_constraint baseStringFamily helper to also recognise
// `universal charstring` (which only the range-bound rule needs).
func baseRangeableStringFamily(spec syntax.TypeSpec) string {
	if base := baseStringFamily(spec); base != "" {
		// length_constraint covers char/bit/hex/octetstring;
		// only charstring is range-eligible per 6.1.2.3.
		if base == "charstring" {
			return base
		}
		return ""
	}
	// universal charstring isn't a single Ident; fall back to
	// stringified form.
	if rs, ok := spec.(*syntax.RefSpec); ok && rs != nil {
		if name := strings.TrimSpace(syntax.Name(rs.X)); name == "universal charstring" {
			return name
		}
	}
	return ""
}

// bogusStringRangeBound flags range bounds that aren't allowed
// for charstring / universal charstring subtypes: infinity is
// undefined for those domains. String literals and char(...)
// quadruples are accepted; non-literal Ident references fall
// through silently.
func bogusStringRangeBound(expr syntax.Expr, base string) *Diagnostic {
	if expr == nil {
		return nil
	}
	if isInfinityExpr(expr) {
		return &Diagnostic{
			Code:     "range-bound-infinity-on-string",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s range bound infinity is not allowed by 6.1.2.3; string range bounds must be character literals",
				base),
		}
	}
	return nil
}

// bogusRangeBound returns a Diagnostic when expr is an illegal
// range bound for `base` (integer / float). Returns nil for
// acceptable shapes (numeric literal, infinity, ident referencing
// a constant we did not resolve, ...). The conservative bias
// matches the rest of the value-constraint code: we only flag
// shapes that are unambiguously wrong.
func bogusRangeBound(expr syntax.Expr, base string) *Diagnostic {
	if expr == nil {
		return nil
	}
	if isInfinityExpr(expr) {
		return nil
	}
	if _, ok := numericValue(expr); ok {
		return nil
	}
	// Boolean / verdict / string literals are unambiguously
	// the wrong shape for a numeric range bound.
	if lit, ok := expr.(*syntax.ValueLiteral); ok && lit.Tok != nil {
		switch lit.Tok.Kind() {
		case syntax.TRUE, syntax.FALSE:
			return &Diagnostic{
				Code:     "range-bound-bool",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s range bound %q is not allowed; range subtyping requires numeric bounds",
					base, lit.Tok.String()),
			}
		case syntax.PASS, syntax.FAIL, syntax.NONE, syntax.INCONC, syntax.ERROR:
			return &Diagnostic{
				Code:     "range-bound-verdict",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s range bound %q is a verdict literal; range subtyping requires numeric bounds",
					base, lit.Tok.String()),
			}
		case syntax.STRING, syntax.BSTRING:
			return &Diagnostic{
				Code:     "range-bound-string",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s range bound %q is a string literal; range subtyping requires numeric bounds",
					base, lit.Tok.String()),
			}
		case syntax.NAN:
			// 6.1.2.3: not_a_number is explicitly excluded
			// from float range bounds.
			return &Diagnostic{
				Code:     "range-bound-nan",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s range bound not_a_number is not allowed by 6.1.2.3",
					base),
			}
		}
	}
	return nil
}

func checkValueInBody(
	body *syntax.BlockStmt,
	tab map[string]valueSpec,
	vars map[string]string,
) []Diagnostic {
	literalInits := collectLiteralInitVars(body)
	compositeInits := collectCompositeInitVars(body)
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if vd, ok := n.(*syntax.ValueDecl); ok {
			diags = append(diags, checkValueDeclInit(vd, tab)...)
			return true
		}
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok {
			return true
		}
		typeName, ok := vars[id.String()]
		if !ok {
			return true
		}
		spec, ok := tab[typeName]
		if !ok {
			return true
		}
		// Element-bound target (`record of integer T (0..10)`):
		// the RHS is a list literal - directly or carried by a
		// variable initialised with one - and each element must
		// satisfy the element constraint. This is the indirect
		// form of NegSem_060302 (`v_list2 := v_list1;` where
		// v_list1 := {2,14,8}); the in-range positive twins keep
		// passing because every element is inside the range.
		if spec.elementBound {
			if cl := compositeLiteralOf(be.Y, compositeInits); cl != nil {
				diags = append(diags, checkElementConstraint(cl, spec, typeName)...)
			}
			return true
		}
		val, fam, ok := numericLiteralValue(be.Y)
		if !ok {
			// Try resolving a bare ident RHS to a known
			// literal initialiser captured in the same body.
			if rid, isId := be.Y.(*syntax.Ident); isId {
				if lit, hasLit := literalInits[rid.String()]; hasLit {
					val, fam, ok = numericLiteralValue(lit)
				}
			}
		}
		if !ok || !spec.familyMatches(fam) || spec.includes(val) {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "value-constraint-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value %s violates %q's constraint %s",
				numericString(val), typeName, spec.describe()),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

// collectLiteralInitVars records every `var T x := <lit>` in body
// whose RHS is a numeric literal. Variables that are reassigned
// later (any plain `x := y`) are removed so we never resolve
// stale literal values.
func collectLiteralInitVars(body syntax.Node) map[string]syntax.Expr {
	out := map[string]syntax.Expr{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			if _, _, ok := numericLiteralValue(dec.Value); !ok {
				continue
			}
			out[dec.Name.String()] = dec.Value
		}
		return true
	})
	if len(out) == 0 {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil ||
			be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		if id, ok := be.X.(*syntax.Ident); ok {
			delete(out, id.String())
		}
		return true
	})
	return out
}

// collectCompositeInitVars records every `var T x := {...}` in body
// whose RHS is a composite (list/record) literal. Variables that are
// reassigned later (any plain `x := y`) are dropped so a stale
// literal is never resolved - mirroring collectLiteralInitVars.
func collectCompositeInitVars(body syntax.Node) map[string]*syntax.CompositeLiteral {
	out := map[string]*syntax.CompositeLiteral{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			if cl, ok := dec.Value.(*syntax.CompositeLiteral); ok && cl != nil {
				out[dec.Name.String()] = cl
			}
		}
		return true
	})
	if len(out) == 0 {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil ||
			be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		if id, ok := be.X.(*syntax.Ident); ok {
			delete(out, id.String())
		}
		return true
	})
	return out
}

// compositeLiteralOf resolves e to a composite literal: either e is
// one directly, or it is a variable initialised with one (captured in
// inits). Returns nil otherwise.
func compositeLiteralOf(e syntax.Expr, inits map[string]*syntax.CompositeLiteral) *syntax.CompositeLiteral {
	switch v := e.(type) {
	case *syntax.CompositeLiteral:
		return v
	case *syntax.Ident:
		if v != nil {
			return inits[v.String()]
		}
	}
	return nil
}

func checkValueDeclInit(vd *syntax.ValueDecl, tab map[string]valueSpec) []Diagnostic {
	id, ok := vd.Type.(*syntax.Ident)
	if !ok {
		return nil
	}
	spec, ok := tab[id.String()]
	if !ok {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		// Element-bound constraints (`type integer A[5]
		// (1..10)`) iterate over the composite-literal
		// elements; scalar constraints validate the RHS as
		// one value.
		if spec.elementBound {
			diags = append(diags, checkElementConstraint(dec.Value, spec, id.String())...)
			continue
		}
		val, fam, ok := numericLiteralValue(dec.Value)
		if !ok || !spec.familyMatches(fam) || spec.includes(val) {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "value-constraint-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value %s violates %q's constraint %s",
				numericString(val), id.String(), spec.describe()),
			Node: dec,
			Span: syntax.SpanOf(dec),
		})
	}
	return diags
}

// checkElementConstraint validates each element of a composite
// literal against an element-bound spec. Non-literal elements
// (function calls, identifiers, sub-templates) fall through
// silently to keep the false-positive rate at zero.
func checkElementConstraint(rhs syntax.Expr, spec valueSpec, typeName string) []Diagnostic {
	cl, ok := rhs.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return nil
	}
	var diags []Diagnostic
	for _, elem := range cl.List {
		if elem == nil {
			continue
		}
		val, fam, ok := numericLiteralValue(elem)
		if !ok || !spec.familyMatches(fam) || spec.includes(val) {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "element-value-constraint-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"element %s violates %q's element constraint %s",
				numericString(val), typeName, spec.describe()),
			Node: elem,
			Span: syntax.SpanOf(elem),
		})
	}
	return diags
}

// valueSpec models a range and/or value-list constraint over a
// numeric base type. Either ranges or values may be populated; an
// includes(v) call succeeds when v matches any entry.
//
// elementBound is true when the constraint applies to each element
// of an array / record-of type rather than to a scalar - e.g.
// `type integer A[5] (1..10);` where each of the 5 cells must be
// in 1..10. The composite-literal initialiser check uses this
// flag to know whether to iterate over the RHS elements or treat
// the whole RHS as a single scalar.
type valueSpec struct {
	base         string // integer / float
	ranges       []valueRange
	values       []float64
	elementBound bool
	// acceptsNaN reports whether the float subtype's value list
	// includes the special `not_a_number` member (NaN). Range
	// membership tests still operate on finite floats - this flag
	// is consulted by includes() when the candidate is NaN.
	acceptsNaN bool
}

type valueRange struct {
	hasLo bool
	lo    float64
	loExc bool // strict-greater-than (lower excluded)
	hasHi bool
	hi    float64
	hiExc bool // strict-less-than (upper excluded)
}

func (v valueSpec) familyMatches(fam string) bool {
	// integer literals are valid in float-typed contexts; the
	// reverse is not the spec's interest here.
	if v.base == "float" && fam == "integer" {
		return true
	}
	return fam == v.base
}

func (v valueSpec) includes(n float64) bool {
	for _, val := range v.values {
		if val == n {
			return true
		}
	}
	for _, r := range v.ranges {
		if r.hasLo {
			if r.loExc && n <= r.lo {
				continue
			}
			if !r.loExc && n < r.lo {
				continue
			}
		}
		if r.hasHi {
			if r.hiExc && n >= r.hi {
				continue
			}
			if !r.hiExc && n > r.hi {
				continue
			}
		}
		return true
	}
	return false
}

func (v valueSpec) describe() string {
	var parts []string
	if v.acceptsNaN {
		parts = append(parts, "not_a_number")
	}
	for _, r := range v.ranges {
		lo := "-infinity"
		if r.hasLo {
			lo = numericString(r.lo)
		}
		hi := "infinity"
		if r.hasHi {
			hi = numericString(r.hi)
		}
		if r.loExc {
			lo = "!" + lo
		}
		if r.hiExc {
			hi = "!" + hi
		}
		parts = append(parts, fmt.Sprintf("(%s..%s)", lo, hi))
	}
	for _, val := range v.values {
		parts = append(parts, numericString(val))
	}
	return strings.Join(parts, ", ")
}

func numericString(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// collectValueConstrainedTypes walks every `type integer T (...)` and
// `type float T (...)` declaration and lifts the parenthesised
// constraint into a valueSpec. The TTCN-3 6.1.2.2 list-of-types
// form `type integer T (T1, T2)` references other subtypes; we
// resolve those after a first literal-only pass via a small
// fixpoint loop so declaration order doesn't matter.
func collectValueConstrainedTypes(mod *syntax.Module) map[string]valueSpec {
	out := map[string]valueSpec{}
	// rawRefs[name] -> list of *Ident referenced from the
	// constraint (e.g. T1, T2 in `(T1, T2)`); we resolve these
	// after the literal-only pass.
	rawRefs := map[string][]string{}
	// rawBase[name] -> base family for any subtype with pending
	// references so the literal-only constraint can carry the
	// base through to the resolved spec.
	rawBase := map[string]string{}
	// rawPartial[name] -> the literal-only ranges/values already
	// captured for this subtype before reference resolution.
	rawPartial := map[string]valueSpec{}

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil ||
			st.Field.ValueConstraint == nil {
			continue
		}
		base, isListElement := numericBaseOrListElement(st.Field.Type)
		if base == "" {
			continue
		}
		spec, refs, ok := parseValueConstraintWithRefs(st.Field.ValueConstraint, base)
		if !ok {
			continue
		}
		// `(not_a_number)` alone is a degenerate "only NaN
		// is valid" spec; without this guard the next bail
		// would drop it.
		if !spec.acceptsNaN && len(spec.ranges) == 0 &&
			len(spec.values) == 0 && len(refs) == 0 {
			continue
		}
		spec.elementBound = len(st.Field.ArrayDef) > 0 || isListElement
		name := st.Field.Name.String()
		if len(refs) == 0 {
			out[name] = spec
			continue
		}
		// Has unresolved type references; record for the
		// fixpoint pass below.
		rawPartial[name] = spec
		rawRefs[name] = refs
		rawBase[name] = base
	}

	// Fixpoint: any subtype whose unresolved references all
	// resolve to specs already in `out` is itself resolved into
	// `out` (literal partial union'd with each referenced spec).
	// Bound at len(rawRefs)+1 iterations because each pass
	// resolves at least one entry; cycles are detected by the
	// loop exiting without progress.
	for changed := true; changed; {
		changed = false
		for name, refs := range rawRefs {
			if _, done := out[name]; done {
				continue
			}
			merged := rawPartial[name]
			merged.base = rawBase[name]
			allResolved := true
			for _, refName := range refs {
				ref, ok := out[refName]
				if !ok {
					allResolved = false
					break
				}
				// Subtype 6.1.2.2 forms a union of the
				// listed types' value sets.
				merged.ranges = append(merged.ranges, ref.ranges...)
				merged.values = append(merged.values, ref.values...)
			}
			if !allResolved {
				continue
			}
			out[name] = merged
			delete(rawRefs, name)
			changed = true
		}
	}
	return out
}

func baseNumericFamily(spec syntax.TypeSpec) string {
	ref, ok := spec.(*syntax.RefSpec)
	if !ok || ref.X == nil {
		return ""
	}
	id, ok := ref.X.(*syntax.Ident)
	if !ok {
		return ""
	}
	switch id.String() {
	case "integer", "float":
		return id.String()
	}
	return ""
}

// numericBaseOrListElement extends baseNumericFamily by also
// recognising `record of <numeric>` / `set of <numeric>` shapes -
// the constraint then applies to each list element rather than to
// a scalar. Returns (base, isListElement). Returns ("", false)
// when the type isn't a numeric scalar or numeric record-of / set-of.
func numericBaseOrListElement(spec syntax.TypeSpec) (string, bool) {
	if base := baseNumericFamily(spec); base != "" {
		return base, false
	}
	ls, ok := spec.(*syntax.ListSpec)
	if !ok || ls == nil || ls.ElemType == nil {
		return "", false
	}
	if base := baseNumericFamily(ls.ElemType); base != "" {
		return base, true
	}
	return "", false
}

func parseValueConstraint(pe *syntax.ParenExpr, base string) (valueSpec, bool) {
	spec, refs, ok := parseValueConstraintWithRefs(pe, base)
	if !ok || len(refs) > 0 {
		// Caller wanted the literal-only form; ignore any
		// type-name references the 6.1.2.2 list-of-types form
		// might have produced.
		if len(refs) > 0 {
			return spec, len(spec.ranges) > 0 || len(spec.values) > 0
		}
		return spec, ok
	}
	return spec, true
}

// parseValueConstraintWithRefs is the lower-level variant that
// returns the literal portion of the constraint alongside any
// type-name references (the list-of-types form `(T1, T2)`). The
// caller resolves the references via a second pass once every
// subtype has been visited.
func parseValueConstraintWithRefs(pe *syntax.ParenExpr, base string) (valueSpec, []string, bool) {
	out := valueSpec{base: base}
	if pe == nil || pe.List == nil {
		return out, nil, false
	}
	var refs []string
	for _, item := range pe.List {
		if be, ok := item.(*syntax.BinaryExpr); ok && be.Op != nil &&
			be.Op.String() == ".." {
			r := valueRange{}
			xExp, loExc := stripExcl(be.X)
			yExp, hiExc := stripExcl(be.Y)
			r.loExc = loExc
			r.hiExc = hiExc
			if isInfinityExpr(xExp) {
				// open lower bound
			} else if lo, ok := numericValue(xExp); ok {
				r.hasLo = true
				r.lo = lo
			} else {
				return valueSpec{}, nil, false
			}
			if isInfinityExpr(yExp) {
				// open upper bound
			} else if hi, ok := numericValue(yExp); ok {
				r.hasHi = true
				r.hi = hi
			} else {
				return valueSpec{}, nil, false
			}
			out.ranges = append(out.ranges, r)
			continue
		}
		if v, ok := numericValue(item); ok {
			out.values = append(out.values, v)
			continue
		}
		// `not_a_number` is the float NaN sentinel. It can
		// appear either as a bare Ident or as a ValueLiteral
		// depending on the parser path; we map both to a
		// dedicated acceptsNaN flag.
		if base == "float" && isNotANumberToken(item) {
			out.acceptsNaN = true
			continue
		}
		// list-of-types form: a bare identifier denotes a
		// subtype whose value set should be unioned into ours.
		if id, ok := item.(*syntax.Ident); ok && id != nil {
			refs = append(refs, id.String())
			continue
		}
		return valueSpec{}, nil, false
	}
	if len(out.ranges) == 0 && len(out.values) == 0 && len(refs) == 0 && !out.acceptsNaN {
		return out, nil, false
	}
	return out, refs, true
}

// isNotANumberToken reports whether e is the `not_a_number`
// float NaN sentinel, accepted regardless of whether the parser
// surfaces it as Ident or ValueLiteral.
func isNotANumberToken(e syntax.Expr) bool {
	if e == nil {
		return false
	}
	switch v := e.(type) {
	case *syntax.Ident:
		return v != nil && v.Tok != nil && v.String() == "not_a_number"
	case *syntax.ValueLiteral:
		return v != nil && v.Tok != nil && v.Tok.String() == "not_a_number"
	}
	return false
}

func numericValue(e syntax.Expr) (float64, bool) {
	if u, ok := e.(*syntax.UnaryExpr); ok && u.Op != nil && u.X != nil {
		if v, ok := numericValue(u.X); ok {
			switch u.Op.Kind() {
			case syntax.SUB:
				return -v, true
			case syntax.ADD:
				return v, true
			}
		}
		return 0, false
	}
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit.Tok == nil {
		return 0, false
	}
	switch lit.Tok.Kind() {
	case syntax.INT:
		n, err := strconv.ParseInt(lit.Tok.String(), 10, 64)
		if err != nil {
			return 0, false
		}
		return float64(n), true
	case syntax.FLOAT:
		f, err := strconv.ParseFloat(lit.Tok.String(), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// numericLiteralValue is the RHS counterpart used by the assignment /
// init checks. It returns (value, family, ok) so callers can confirm
// the literal matches the constrained type's base family.
func numericLiteralValue(e syntax.Expr) (float64, string, bool) {
	if u, ok := e.(*syntax.UnaryExpr); ok && u.Op != nil && u.X != nil {
		val, fam, ok := numericLiteralValue(u.X)
		if !ok {
			return 0, "", false
		}
		switch u.Op.Kind() {
		case syntax.SUB:
			return -val, fam, true
		case syntax.ADD:
			return val, fam, true
		}
		return 0, "", false
	}
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit.Tok == nil {
		return 0, "", false
	}
	switch lit.Tok.Kind() {
	case syntax.INT:
		n, err := strconv.ParseInt(lit.Tok.String(), 10, 64)
		if err != nil {
			return 0, "", false
		}
		return float64(n), "integer", true
	case syntax.FLOAT:
		f, err := strconv.ParseFloat(lit.Tok.String(), 64)
		if err != nil {
			return 0, "", false
		}
		return f, "float", true
	}
	return 0, "", false
}

// isInfinityExpr returns true for the literal forms TTCN-3 uses to
// denote unbounded range endpoints: `infinity`, `-infinity` and
// `+infinity`.
func isInfinityExpr(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.UnaryExpr:
		if x.Op == nil || x.X == nil {
			return false
		}
		switch x.Op.Kind() {
		case syntax.SUB, syntax.ADD:
			return isInfinityExpr(x.X)
		}
	case *syntax.Ident:
		if x.Tok == nil {
			return false
		}
		return strings.EqualFold(x.String(), "infinity")
	}
	return false
}

func collectLocalValueConstrainedVars(body syntax.Node, tab map[string]valueSpec) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok {
			return true
		}
		id, ok := vd.Type.(*syntax.Ident)
		if !ok {
			return true
		}
		if _, ok := tab[id.String()]; !ok {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = id.String()
		}
		return true
	})
	return out
}
