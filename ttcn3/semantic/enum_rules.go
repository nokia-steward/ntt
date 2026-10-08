// enum_rules.go enforces ETSI ES 201 873-1 clauses 6.2.4 / 6.2.7
// on enumerated type declarations:
//
//   - enumerator identifiers must be unique within the type
//     (NegSem_060204_001);
//   - user-assigned integer values must be unique within the
//     type (NegSem_060204_002).
//
// The implementation parses each enumerator into either a bare
// identifier or a `Name(value)` call shape. Anything more
// exotic is silently ignored to keep the false-positive rate at
// zero.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkEnumRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		ed, ok := d.Def.(*syntax.EnumTypeDecl)
		if !ok || ed.Name == nil || len(ed.Enums) == 0 {
			continue
		}
		typeName := ed.Name.String()
		seenName := map[string]bool{}
		seenValue := map[int64]bool{}
		for _, e := range ed.Enums {
			if shape := badEnumeratorShape(e); shape != "" {
				diags = append(diags, Diagnostic{
					Code:     "enum-user-value-not-literal",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s in %s: user-assigned enumerator value must be a non-empty list of integer literals or ranges (ETSI 6.2.4)",
						shape, typeName),
					Node: e,
					Span: syntax.SpanOf(e),
				})
				continue
			}
			name, val, hasVal, anchor := enumeratorParts(e)
			if name == "" {
				continue
			}
			if seenName[name] {
				diags = append(diags, Diagnostic{
					Code:     "enum-duplicate-identifier",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"enumerator %q is declared more than once in %s (ETSI 6.2.7)",
						name, typeName),
					Node: anchor,
					Span: syntax.SpanOf(anchor),
				})
				continue
			}
			seenName[name] = true
			if !hasVal {
				continue
			}
			if seenValue[val] {
				diags = append(diags, Diagnostic{
					Code:     "enum-duplicate-value",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"user-assigned value %d is reused in %s (ETSI 6.2.4)",
						val, typeName),
					Node: anchor,
					Span: syntax.SpanOf(anchor),
				})
				continue
			}
			seenValue[val] = true
		}
	}
	return diags
}

// badEnumeratorShape returns a non-empty description when the
// enumerator's user-assigned value list is illegal per ETSI 6.2.4
// v4.13.1 (revised by STF 572):
//
//   - Single argument: any integer expression is allowed (incl.
//     a bare constant reference or `2 + 3`). We only reject the
//     empty list here.
//   - Multiple arguments: each list element must be an integer
//     literal or a literal-bounded range (no constant refs, no
//     compound expressions).
//   - Zero arguments (`Tuesday()`): always rejected.
func badEnumeratorShape(e syntax.Expr) string {
	ce, ok := e.(*syntax.CallExpr)
	if !ok || ce == nil || ce.Fun == nil {
		return ""
	}
	id, ok := ce.Fun.(*syntax.Ident)
	if !ok || id == nil {
		return ""
	}
	name := id.String()
	if ce.Args == nil || len(ce.Args.List) == 0 {
		return fmt.Sprintf("enumerator %q has empty user-value list", name)
	}
	if len(ce.Args.List) == 1 {
		return ""
	}
	for _, arg := range ce.Args.List {
		if !isIntLiteralOrRange(arg) {
			return fmt.Sprintf("enumerator %q has a non-literal in its user-value list", name)
		}
	}
	return ""
}

// isIntLiteralOrRange is true for an integer literal, a signed
// integer literal (UnaryExpr with `-` op), or a BinaryExpr `..`
// range whose endpoints are themselves integer literals.
func isIntLiteralOrRange(e syntax.Expr) bool {
	if _, ok := signedIntLiteral(e); ok {
		return true
	}
	if be, ok := e.(*syntax.BinaryExpr); ok && be != nil && be.Op != nil &&
		be.Op.String() == ".." {
		if _, ok := signedIntLiteral(be.X); !ok {
			return false
		}
		if _, ok := signedIntLiteral(be.Y); !ok {
			return false
		}
		return true
	}
	return false
}

// enumeratorParts pulls (name, value, hasValue, anchor) out of
// an enumerator expression. Two shapes are recognised:
//
//   - `Ident` -> bare enumerator with no user value;
//   - `Ident(intLiteral)` -> user-assigned enumerator value.
//
// The anchor is the node we point diagnostics at; we prefer the
// inner Ident over the CallExpr so the message lands on the
// enumerator name.
func enumeratorParts(e syntax.Expr) (string, int64, bool, syntax.Node) {
	switch v := e.(type) {
	case *syntax.Ident:
		if v == nil {
			return "", 0, false, nil
		}
		return v.String(), 0, false, v
	case *syntax.CallExpr:
		if v == nil || v.Fun == nil || v.Args == nil ||
			len(v.Args.List) != 1 {
			return "", 0, false, nil
		}
		id, ok := v.Fun.(*syntax.Ident)
		if !ok || id == nil {
			return "", 0, false, nil
		}
		val, ok := signedIntLiteral(v.Args.List[0])
		if !ok {
			return id.String(), 0, false, id
		}
		return id.String(), val, true, id
	}
	return "", 0, false, nil
}
