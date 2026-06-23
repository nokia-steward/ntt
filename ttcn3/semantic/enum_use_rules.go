// enum_use_rules.go enforces ETSI ES 201 873-1 clause 6.2.4 /
// 6.2.7 on the *use* sites (as opposed to *declaration* sites
// covered by enum_rules.go) of enumerated types:
//
//   - `var EnumT v := 0;`: assigning a bare integer literal to a
//     variable of an enumerated type is forbidden.  Enumerator
//     ordinals are an implementation detail and can only be reached
//     through int2enum / enum2int (NegSem_060204_003).
//   - `var EnumT v := Friday(5);`: the `Name(value)` enumerator
//     syntax is only allowed in a value position when the
//     enumerator was declared with a *range* or *value list* (the
//     literal selects one of the values from the set).  Repeating
//     the single literal of a fixed-value enumerator (e.g.
//     `Friday(5)` after `Friday(5)`) or supplying a literal for a
//     bare enumerator is forbidden (NegSem_060204_013).
//
// The rule is intentionally name-based and does not run an actual
// type pass.  It collects every enum type and enumerator declared
// in the module, then walks initialisers and assignments looking
// for the two illegal shapes above.  The walk skips CallExpr nodes
// that *are* the enumerator definition itself so we don't yell at
// the declaration list.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkEnumUseRules(mod *syntax.Module) []Diagnostic {
	enumTypes := map[string]bool{}
	enumLabels := map[string]bool{}
	// fixedEnumNames lists enumerator identifiers whose declared
	// value is a single literal or no literal at all - i.e. the
	// ordinal slot is fixed by the declaration and `Name(value)`
	// in a value position is not allowed.  Enumerators declared
	// with a range or value list are intentionally left out so
	// `Friday(15)` after `Friday(11..30)` is still accepted (see
	// Sem_060204_enumerated_type_and_values_007).
	fixedEnumNames := map[string]bool{}
	// rangedEnumNames remembers enumerator identifiers declared
	// with a range / multi-element value list anywhere in the
	// module.  When the same identifier appears with both a fixed
	// and a ranged shape (separate enum types reusing the same
	// label) we can't tell which one the user meant at the call
	// site, so we fall back to "accept" by erasing the fixed flag.
	rangedEnumNames := map[string]bool{}
	defCalls := map[*syntax.CallExpr]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		ed, ok := d.Def.(*syntax.EnumTypeDecl)
		if !ok || ed.Name == nil {
			continue
		}
		enumTypes[ed.Name.String()] = true
		for _, e := range ed.Enums {
			switch v := e.(type) {
			case *syntax.Ident:
				if v != nil && v.Tok != nil {
					name := v.Tok.String()
					fixedEnumNames[name] = true
					enumLabels[name] = true
				}
			case *syntax.CallExpr:
				if v == nil {
					continue
				}
				defCalls[v] = true
				id, ok := v.Fun.(*syntax.Ident)
				if !ok || id == nil || id.Tok == nil {
					continue
				}
				name := id.Tok.String()
				enumLabels[name] = true
				if enumeratorIsFixedShape(v) {
					fixedEnumNames[name] = true
				} else {
					rangedEnumNames[name] = true
				}
			}
		}
	}
	if len(enumTypes) == 0 {
		return nil
	}
	for name := range rangedEnumNames {
		delete(fixedEnumNames, name)
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			diags = append(diags, checkEnumVarInit(x, enumTypes)...)
		}
		diags = append(diags, checkEnumOrdinalCall(n, fixedEnumNames, defCalls)...)
		diags = append(diags, checkBareEnumComparison(n, enumLabels)...)
		return true
	})
	return diags
}

// enumeratorIsFixedShape returns true when the enumerator's value
// list is a single signed integer literal (i.e. exactly one fixed
// ordinal).  Multi-element lists and ranges return false because
// those declarations let value-position references pick one of the
// declared values via `Name(value)`.
func enumeratorIsFixedShape(ce *syntax.CallExpr) bool {
	if ce == nil || ce.Args == nil || len(ce.Args.List) != 1 {
		return false
	}
	_, ok := signedIntLiteral(ce.Args.List[0])
	return ok
}

// checkEnumVarInit fires when `var EnumT v := <int literal>;`.
func checkEnumVarInit(
	vd *syntax.ValueDecl,
	enumTypes map[string]bool,
) []Diagnostic {
	if vd == nil {
		return nil
	}
	typeName := identName(vd.Type)
	if typeName == "" || !enumTypes[typeName] {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		if _, ok := signedIntLiteral(dec.Value); !ok {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "enum-init-with-integer",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"cannot assign a bare integer literal to enumerated type %q (use int2enum, ETSI 6.2.4)",
				typeName),
			Node: dec.Value,
			Span: syntax.SpanOf(dec.Value),
		})
	}
	return diags
}

// checkEnumOrdinalCall flags `EnumName(int)` reused outside the
// enum declaration list when EnumName is a fixed-shape enumerator.
// Range / value-list enumerators are intentionally accepted (see
// Sem_060204_enumerated_type_and_values_007).
func checkEnumOrdinalCall(
	n syntax.Node,
	fixedEnumNames map[string]bool,
	defCalls map[*syntax.CallExpr]bool,
) []Diagnostic {
	ce, ok := n.(*syntax.CallExpr)
	if !ok || ce == nil || defCalls[ce] {
		return nil
	}
	id, ok := ce.Fun.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return nil
	}
	name := id.Tok.String()
	if !fixedEnumNames[name] {
		return nil
	}
	if ce.Args == nil || len(ce.Args.List) != 1 {
		return nil
	}
	if _, ok := signedIntLiteral(ce.Args.List[0]); !ok {
		return nil
	}
	return []Diagnostic{{
		Code:     "enum-ordinal-in-value-ref",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"enumerator %q has no value list - cannot supply an ordinal in a value reference (ETSI 6.2.7)",
			name),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}}
}

// checkBareEnumComparison rejects comparisons like `Tuesday != Wednesday`.
// Neither side provides the required implicit or explicit enum type context.
// A typed operand (`v_day == Monday`) is accepted because the variable's
// declared type supplies that context.
func checkBareEnumComparison(n syntax.Node, enumLabels map[string]bool) []Diagnostic {
	be, ok := n.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil {
		return nil
	}
	switch be.Op.Kind() {
	case syntax.EQ, syntax.NE:
	default:
		return nil
	}
	lhs := identName(be.X)
	rhs := identName(be.Y)
	if lhs == "" || rhs == "" {
		return nil
	}
	if !enumLabels[lhs] || !enumLabels[rhs] {
		return nil
	}
	return []Diagnostic{{
		Code:     "bare-enum-comparison-without-type-context",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"comparison `%s %s %s` uses enumerated values without an implicit or explicit type reference (ETSI 6.2.4)",
			lhs, be.Op.String(), rhs),
		Node: be,
		Span: syntax.SpanOf(be),
	}}
}
