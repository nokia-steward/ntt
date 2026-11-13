// template_range_rules.go enforces ETSI ES 201 873-1 clause
// B.1.2.5 (Value Range matching mechanism) on the *shape* of
// range bounds appearing in template definitions:
//
//   - template-range-reversed: the lower bound is strictly greater
//     than the upper bound (e.g. `(2..0)` or `("fff".."aaa")`),
//     which produces an empty match set and is almost certainly a
//     typo.
//   - template-range-on-enum: a range bound applied to a field
//     whose declared type is an enumeration. Enumerated values are
//     not orderable per 6.1.2.3 / B.1.2.5, so range matching is
//     undefined on them.
//
// We only inspect ParenExpr value-range literals (`(lo..hi)`)
// directly embedded in CompositeLiteral entries or as the entire
// template body; anything more indirect (function calls,
// references) is left to the runtime / value resolver.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkTemplateRangeRules is wired into Analyze; emits the two
// diagnostics described above.
func (a *Analyzer) checkTemplateRangeRules(mod *syntax.Module) []Diagnostic {
	enumTypes := collectEnumTypeNames(mod)
	fieldTyp := collectStructFieldDeclTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			if x == nil || x.Value == nil {
				return true
			}
			typeName := identName(x.Type)
			diags = append(diags,
				inspectTemplateValue(x.Value, typeName, fieldTyp, enumTypes)...)
		case *syntax.ValueDecl:
			if x == nil {
				return true
			}
			typeName := identName(x.Type)
			for _, dec := range x.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				diags = append(diags,
					inspectTemplateValue(dec.Value, typeName, fieldTyp, enumTypes)...)
			}
		}
		return true
	})
	return diags
}

// inspectTemplateValue walks `val` looking for value-range
// constructs. typeName is the declared type of the value (record /
// set name for a CompositeLiteral; field type when recursing).
func inspectTemplateValue(
	val syntax.Expr,
	typeName string,
	fieldTyp map[string]structFieldTyp,
	enumTypes map[string]bool,
) []Diagnostic {
	var diags []Diagnostic
	// Direct range as the whole template value.
	if rng, ok := asRange(val); ok {
		diags = append(diags, checkRangeShape(rng, typeName, enumTypes, "")...)
		return diags
	}
	cl, ok := val.(*syntax.CompositeLiteral)
	if !ok {
		return diags
	}
	fields := fieldTyp[typeName]
	for _, item := range cl.List {
		be, ok := item.(*syntax.BinaryExpr)
		if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			continue
		}
		fieldName := identName(be.X)
		if fieldName == "" {
			continue
		}
		fieldTypeName := ""
		if fields != nil {
			fieldTypeName = fields[fieldName]
		}
		if rng, ok := asRange(be.Y); ok {
			diags = append(diags,
				checkRangeShape(rng, fieldTypeName, enumTypes, fieldName)...)
			continue
		}
		// Recurse into nested composite literals using the
		// field's declared struct type as the new context.
		if nested, ok := be.Y.(*syntax.CompositeLiteral); ok && fieldTypeName != "" {
			diags = append(diags,
				inspectTemplateValue(nested, fieldTypeName, fieldTyp, enumTypes)...)
		}
	}
	return diags
}

// asRange returns the `lo .. hi` BinaryExpr inside a ParenExpr
// when val is `(lo .. hi)`. Open ranges (`(lo..)` / `(..hi)`) are
// not in scope; checkRangeShape handles whatever lo / hi the
// parser handed back.
func asRange(val syntax.Expr) (*syntax.BinaryExpr, bool) {
	pe, ok := val.(*syntax.ParenExpr)
	if !ok || pe == nil || len(pe.List) != 1 {
		return nil, false
	}
	be, ok := pe.List[0].(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil {
		return nil, false
	}
	if be.Op.String() != ".." {
		return nil, false
	}
	return be, true
}

// checkRangeShape applies the reversed-bound and range-on-enum
// rules to a single `(lo..hi)` range.
func checkRangeShape(
	rng *syntax.BinaryExpr,
	targetType string,
	enumTypes map[string]bool,
	fieldName string,
) []Diagnostic {
	var diags []Diagnostic

	if enumTypes[targetType] {
		diags = append(diags, Diagnostic{
			Code:     "template-range-on-enum",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value range (lo..hi) is not allowed on enumerated type %q (context: %s)",
				targetType, fieldOrTemplate(fieldName)),
			Node: rng,
			Span: syntax.SpanOf(rng),
		})
		return diags
	}
	// Reversed-bound rule: only fires when both sides are
	// comparable literal forms (numeric or string) and lo > hi.
	lo, _ := stripExcl(rng.X)
	hi, _ := stripExcl(rng.Y)
	if rangeBoundsReversed(lo, hi) {
		diags = append(diags, Diagnostic{
			Code:     "template-range-reversed",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value range (lo..hi) bounds are in reverse order (context: %s)",
				fieldOrTemplate(fieldName)),
			Node: rng,
			Span: syntax.SpanOf(rng),
		})
	}
	return diags
}

// rangeBoundsReversed compares lo / hi when both are recognisable
// literals. Returns false for any shape we can't compare directly.
func rangeBoundsReversed(lo, hi syntax.Expr) bool {
	if lo == nil || hi == nil {
		return false
	}
	if loN, ok := numericValueLit(lo); ok {
		if hiN, ok := numericValueLit(hi); ok {
			return loN > hiN
		}
	}
	if loS, ok := stringLiteralValue(lo); ok {
		if hiS, ok := stringLiteralValue(hi); ok {
			return strings.Compare(loS, hiS) > 0
		}
	}
	return false
}

// numericValueLit is a thin wrapper around the numericValue
// helper in value_constraint.go that returns -infinity / +infinity
// as math.Inf with the correct sign; here we only care about
// finite numbers so we discard infinities.
func numericValueLit(e syntax.Expr) (float64, bool) {
	if isInfinityExpr(e) {
		return 0, false
	}
	return numericValue(e)
}

// stringLiteralValue returns the underlying string when e is a
// STRING ValueLiteral, stripped of its surrounding quotes.
func stringLiteralValue(e syntax.Expr) (string, bool) {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return "", false
	}
	if lit.Tok.Kind() != syntax.STRING {
		return "", false
	}
	s := lit.Tok.String()
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1], true
	}
	return s, true
}

// fieldOrTemplate produces a "<field>" / "template body" locus
// string for diagnostic messages.
func fieldOrTemplate(field string) string {
	if field == "" {
		return "template body"
	}
	return fmt.Sprintf("field %q", field)
}

// collectEnumTypeNames returns the set of top-level enumerated
// type names declared in the module.
func collectEnumTypeNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		if et, ok := d.Def.(*syntax.EnumTypeDecl); ok && et != nil && et.Name != nil {
			out[et.Name.String()] = true
		}
	}
	return out
}
