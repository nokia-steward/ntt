// all_from_source_rules.go enforces ETSI ES 201 873-1 clauses
// B.1.2.1 / B.1.2.6 / B.1.2.7 restrictions on the `all from`
// matching clause.
//
// `(all from src)`, `superset(all from src)` and `subset(all
// from src)` MUST refer to a template (or value) whose body
// contains only plain values - the per-element matching
// mechanisms `*` (AnyElementsOrNone), `permutation(...)`,
// `omit`, nested `superset(...)` and nested `subset(...)` are
// forbidden in the source template.
//
// Inside `permutation(...)` B.1.3.3 is laxer: the source may be a
// combined template, `*` included, as long as it contains no
// permutation (restriction c); its member type shall be compatible
// with the permutation's (restriction b,
// all-from-member-type-incompatible, for predefined member types).
//
// We resolve the source by name to the matching module-level
// `template T name := ...` declaration, then inspect the body
// for the forbidden shapes. Anything we can't resolve (cross-
// module references, parameterised templates, computed sources)
// falls through silently to keep the false-positive rate at
// zero.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAllFromSourceRules(mod *syntax.Module) []Diagnostic {
	bodies := collectModuleTemplateBodies(mod)
	if len(bodies) == 0 {
		return nil
	}
	inPerm := allFromInPermutation(mod)
	tmplTypes := collectTemplateTypeNames(mod)
	elems := collectListElementTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fe, ok := n.(*syntax.FromExpr)
		if !ok || fe == nil || fe.KindTok == nil {
			return true
		}
		if fe.KindTok.String() != "all" {
			return true
		}
		id, ok := fe.X.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		body, ok := bodies[id.String()]
		if !ok {
			return true
		}
		permType, isPerm := inPerm[fe]
		var bad string
		if isPerm {
			// B.1.3.3 c: inside a permutation the source may be a
			// combined template, `*` included, without permutations.
			bad = forbiddenPermutationAllFrom(body)
			if bad == "" {
				want, have := elems[permType], elems[tmplTypes[id.String()]]
				if incompatiblePrimitiveTypes(want, have) {
					diags = append(diags, Diagnostic{
						Code:     "all-from-member-type-incompatible",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"`all from %s` in a permutation of %s elements: member type %s is not compatible (ETSI B.1.3.3 b)",
							id.String(), want, have),
						Node: fe,
						Span: syntax.SpanOf(fe),
					})
				}
			}
		} else {
			bad = forbiddenAllFromElement(body)
		}
		if bad == "" {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "all-from-source-has-matching-mechanism",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`all from %s`: source template contains forbidden matching mechanism %s (ETSI B.1.2.{1,6,7})",
				id.String(), bad),
			Node: fe,
			Span: syntax.SpanOf(fe),
		})
		return true
	})
	return diags
}

// allFromInPermutation maps each `all from` listed directly in a
// permutation(...) to the type name of the template declaring the
// permutation ("" when it is not a named template declaration).
func allFromInPermutation(mod *syntax.Module) map[*syntax.FromExpr]string {
	out := map[*syntax.FromExpr]string{}
	mark := func(root syntax.Node, ty string) {
		syntax.Inspect(root, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil || ce.Args == nil || identName(ce.Fun) != "permutation" {
				return true
			}
			for _, arg := range ce.Args.List {
				if fe, ok := arg.(*syntax.FromExpr); ok && fe != nil {
					out[fe] = ty
				}
			}
			return true
		})
	}
	mark(mod, "")
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if td, ok := n.(*syntax.TemplateDecl); ok && td != nil && td.Value != nil {
			mark(td.Value, identName(td.Type))
		}
		return true
	})
	return out
}

// forbiddenPermutationAllFrom is forbiddenAllFromElement for an
// `all from` inside a permutation: the source as a whole shall be a
// specific value or a combined template, and no element of it a
// permutation (B.1.3.3 c).
func forbiddenPermutationAllFrom(body syntax.Expr) string {
	if lbl := classifyAllFromMember(body); lbl != "" {
		return lbl
	}
	cl, ok := body.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return ""
	}
	for _, item := range cl.List {
		if ce, ok := item.(*syntax.CallExpr); ok && identName(ce.Fun) == "permutation" {
			return "`permutation(...)`"
		}
	}
	return ""
}

// incompatiblePrimitiveTypes reports whether a and b are different
// predefined types neither of which is compatible with the other
// (6.3.1); charstring values are compatible with universal charstring.
func incompatiblePrimitiveTypes(a, b string) bool {
	if a == b || !isPrimitiveType(a) || !isPrimitiveType(b) {
		return false
	}
	strs := map[string]bool{"charstring": true, "universal": true, "universal charstring": true}
	return !(strs[a] && strs[b])
}

// collectModuleTemplateBodies maps every `template T name :=
// body` (both module-level and locally declared inside function /
// testcase / altstep bodies) to its Value expression.
// Parameterised templates are skipped because we don't pin down
// which actuals would be in play at the call site.
func collectModuleTemplateBodies(mod *syntax.Module) map[string]syntax.Expr {
	out := map[string]syntax.Expr{}
	if mod == nil {
		return out
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil || td.Value == nil {
			return true
		}
		if td.Params != nil && len(td.Params.List) > 0 {
			return true
		}
		out[td.Name.String()] = td.Value
		return true
	})
	return out
}

// forbiddenAllFromElement returns a short label naming the first
// forbidden matching mechanism it finds inside body, or "" if
// none.  Both shapes the suite uses are handled:
//
//   - `template T name := { ...members... }` - walk the literal.
//   - `template T name := superset(...)` / `subset(...)` - the
//     whole template body IS the matching mechanism, which is
//     itself a forbidden member when referenced from `all from`.
func forbiddenAllFromElement(body syntax.Expr) string {
	if lbl := classifyAllFromMember(body); lbl != "" {
		return lbl
	}
	cl, ok := body.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return ""
	}
	for _, item := range cl.List {
		if lbl := classifyAllFromMember(item); lbl != "" {
			return lbl
		}
	}
	return ""
}

// classifyAllFromMember inspects a single template-element entry
// (or the whole template body) and reports its kind when it is
// one of the forbidden shapes.
func classifyAllFromMember(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		if v == nil || v.Tok == nil {
			return ""
		}
		switch v.Tok.String() {
		case "*":
			return "`*` (AnyElementsOrNone)"
		case "omit":
			return "`omit`"
		}
	case *syntax.Ident:
		if v != nil && v.Tok != nil && v.String() == "omit" {
			return "`omit`"
		}
	case *syntax.CallExpr:
		name := identName(v.Fun)
		switch name {
		case "permutation":
			return "`permutation(...)`"
		case "superset":
			return "`superset(...)`"
		case "subset":
			return "`subset(...)`"
		}
	}
	return ""
}
