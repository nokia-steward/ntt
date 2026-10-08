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
		bad := forbiddenAllFromElement(body)
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
