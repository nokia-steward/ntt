// type_compat_rules.go enforces ETSI ES 201 873-1 clause 6.3.2
// "Type compatibility of structured types". These are whole-value
// assignment checks between two locally-declared variables whose
// declared types place them in incompatible type classes:
//
//   - 6.3.2 / G.9: a struct-family value (record / set / union)
//     and a collection-family value (record of / set of / array)
//     are never compatible, regardless of element count or order
//     (NegSem_060302_014..018). Note this holds even when the
//     arities match (018: `set {a,b,c}` vs `integer[3]`), because
//     `set`/`record` are distinct type classes from `set of` /
//     `record of` / array.
//   - 6.3.2: a union value is compatible with another union only
//     if same-named alternatives have compatible types; here we
//     catch the enum-alternative mismatch (NegSem_060302_009)
//     while honouring enum synonyms so the compatible sibling
//     (Sem_060302_001, whose alternative uses an enum synonym)
//     keeps passing.
//
// The checks are deliberately local: both sides must be bare
// identifiers resolvable to a var/const declared in the same
// function/testcase/altstep body. Field/index targets, formal
// parameters and cross-module types are left alone to avoid false
// positives.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTypeCompatRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	typeCats := collectTypeCategories(mod)
	enumRoots := collectEnumTypeNames(mod)
	aliases := collectTypeAliasBases(mod)
	structFields := collectStructFieldTypes(mod)

	canonical := func(t string) string { return canonicalTypeName(t, aliases) }

	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		varTypes := collectFuncBodyVarTypes(fn.Body)
		arrayDims := collectFuncBodyArrayDims(fn.Body)

		catOf := func(name string) (typeCategory, bool) {
			if arrayDims[name] > 0 {
				return typeCategoryArray, true
			}
			ty, ok := varTypes[name]
			if !ok {
				return typeCategoryOther, false
			}
			if c, ok := typeCats[ty]; ok {
				return c, true
			}
			return typeCategoryOther, true
		}

		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			be, ok := n.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil ||
				be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhs := identName(be.X)
			rhs := identName(be.Y)
			if lhs == "" || rhs == "" {
				return true
			}
			lc, lok := catOf(lhs)
			rc, rok := catOf(rhs)
			if !lok || !rok {
				return true
			}

			// 6.3.2 / G.9: struct family vs collection family.
			if (isStructFamily(lc) && isCollectionFamily(rc)) ||
				(isCollectionFamily(lc) && isStructFamily(rc)) {
				diags = append(diags, Diagnostic{
					Code:     "incompatible-struct-collection-assign",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"incompatible assignment: %q (%s) and %q (%s) are different type classes; record/set/union are not compatible with record of/set of/array (ETSI 6.3.2)",
						lhs, lc, rhs, rc),
					Node: be,
					Span: syntax.SpanOf(be),
				})
				return true
			}

			lt := varTypes[lhs]
			rt := varTypes[rhs]

			// NOTE: a standalone "distinct enumerated types" rule
			// (NegSem_060302_001) is intentionally NOT implemented.
			// Sem_060302_009 is byte-identical code (two
			// independent `type enumerated {e_black,e_white}` and
			// `v_enum2 := v_enum1`) yet is annotated `pass`, so no
			// static rule can distinguish the two; rejecting one
			// regresses the other (a documented poisoned well).

			// 6.3.2: union alternative enum-type mismatch.
			if lc == typeCategoryUnion && rc == typeCategoryUnion {
				if alt := unionAltEnumMismatch(
					structFields, lt, rt, canonical, enumRoots); alt != "" {
					diags = append(diags, Diagnostic{
						Code:     "incompatible-union-alt-assign",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"incompatible assignment: union %q and %q disagree on the type of alternative %q (incompatible enumerated types) (ETSI 6.3.2)",
							lt, rt, alt),
						Node: be,
						Span: syntax.SpanOf(be),
					})
				}
			}
			return true
		})
	}
	return diags
}

// isStructFamily reports whether c is a named structured type with
// named fields/alternatives (record / set / union).
func isStructFamily(c typeCategory) bool {
	return c == typeCategoryRecord ||
		c == typeCategorySet ||
		c == typeCategoryUnion
}

// isCollectionFamily reports whether c is an ordered/unordered
// homogeneous collection (record of / set of / array).
func isCollectionFamily(c typeCategory) bool {
	return c == typeCategoryRecordOf ||
		c == typeCategorySetOf ||
		c == typeCategoryArray
}

// collectTypeAliasBases maps every `type Base Alias;` synonym
// declaration to its immediate base type name. Only the pure
// alias shape (a SubTypeDecl whose Field.Type is a bare type
// reference) is recorded; constrained subtypes (`type integer I
// (0..10);`) and container subtypes are intentionally excluded -
// they are handled by collectTypeCategories.
func collectTypeAliasBases(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil {
			continue
		}
		if len(st.Field.ArrayDef) > 0 {
			continue
		}
		rs, ok := st.Field.Type.(*syntax.RefSpec)
		if !ok || rs == nil {
			continue
		}
		base := identName(rs.X)
		if base == "" {
			continue
		}
		out[st.Field.Name.String()] = base
	}
	return out
}

// canonicalTypeName follows the synonym alias chain from name to
// its root type name. A cycle guard caps the walk at the number of
// known aliases.
func canonicalTypeName(name string, aliases map[string]string) string {
	seen := map[string]bool{}
	for {
		base, ok := aliases[name]
		if !ok || base == "" || seen[name] {
			return name
		}
		seen[name] = true
		name = base
	}
}

// unionAltEnumMismatch returns the first alternative name shared by
// union types lt and rt whose alternative types are both enumerated
// but resolve to different canonical enum roots. It returns "" when
// the alternatives are compatible (including the synonym case).
func unionAltEnumMismatch(
	structFields map[string]fieldsOfStruct,
	lt, rt string,
	canonical func(string) string,
	enumRoots map[string]bool,
) string {
	lf := structFields[lt]
	rf := structFields[rt]
	if lf == nil || rf == nil {
		return ""
	}
	for name, lspec := range lf {
		rspec, ok := rf[name]
		if !ok {
			continue
		}
		la := canonical(lspec.typeName)
		ra := canonical(rspec.typeName)
		if la == "" || ra == "" {
			continue
		}
		if enumRoots[la] && enumRoots[ra] && la != ra {
			return name
		}
	}
	return ""
}
