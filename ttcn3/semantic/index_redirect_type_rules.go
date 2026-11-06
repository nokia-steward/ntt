// index_redirect_type_rules.go enforces the element-type half of the
// `@index value v` redirect restriction (ETSI ES 201 873-1 clauses
// 21.3.2 / 22.{2,3}): the index target shall be integer-typed (a
// scalar integer for a 1-D source, a record-of integer for a
// multi-dimensional one).
//
// checkIndexRedirectDimRules already validates the array *shape* for
// port-array sources. This complementary rule catches a scalar target
// declared with a primitive type that can never hold an index value
// (e.g. `var float v_index; ... -> @index value v_index`), which also
// applies to component-array sources (`any from v_ptc.alive`) that the
// port-only dim rule does not see.
//
// The check is intentionally conservative: it only fires for a scalar
// (non-array) target whose declared type is a well-known primitive
// that is definitely not integer-compatible, so integer subtypes and
// anything the analyzer cannot resolve are left untouched.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkIndexRedirectTargetTypeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		dims := collectFuncBodyArrayDims(fn.Body)
		types := collectFuncBodyVarTypes(fn.Body)

		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			r, ok := n.(*syntax.RedirectExpr)
			if !ok || r == nil || r.IndexTok == nil || r.Index == nil {
				return true
			}
			target := identName(r.Index)
			if target == "" {
				return true
			}
			// Only scalar targets: a `var T v[..]` array target is
			// covered (shape-wise) by the dim rule and may legitimately
			// be a record-of integer.
			if dims[target] != 0 {
				return true
			}
			ty, known := types[target]
			if !known || !isDefinitelyNonIntegerScalar(ty) {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "index-redirect-target-not-integer",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"@index value %s: target is of type `%s`; the index redirect target must be integer-typed (ETSI 21.3.2 / 22.{2,3})",
					target, ty),
				Node: r,
				Span: syntax.SpanOf(r),
			})
			return true
		})
	}
	return diags
}

// isDefinitelyNonIntegerScalar reports whether ty is a well-known
// primitive type that can never serve as an `@index value` target.
// Restricted to unambiguous primitives so integer subtypes and
// unresolved type names are never falsely rejected.
func isDefinitelyNonIntegerScalar(ty string) bool {
	switch ty {
	case "float", "boolean", "charstring", "universal charstring",
		"octetstring", "hexstring", "bitstring", "verdicttype":
		return true
	}
	return false
}
