// struct_field_range_rules.go extends the 6.1.2.3 value-constraint
// check across the structured-type compatibility boundary of ETSI
// 6.3.2: when a record / set / union value is assigned to a variable
// of a different (but structurally compatible) type, every field
// value must still satisfy the *target* field's range subtype.
//
// record / set fields map positionally (6.3.2.2): the i-th source
// field value is checked against the i-th target field's constraint,
// so the source literal's names are resolved to their declared
// position in the source type. union alternatives map by name: the
// chosen alternative is checked against the target's same-named
// alternative.
//
// This catches NegSem_060302_structured_types_003 / 005 / 008
// (`v_rec2 := v_rec1;` where v_rec1 carries an out-of-range field
// value). It only fires on numeric literal field values that fall
// outside a numeric target constraint, so the in-range positive twins
// are untouched - the same zero-false-positive bias as
// value_constraint.go.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type fieldConstraint struct {
	name    string
	hasSpec bool
	spec    valueSpec
}

type structFieldInfo struct {
	isUnion bool
	fields  []fieldConstraint
}

func (a *Analyzer) checkStructFieldRangeRules(mod *syntax.Module) []Diagnostic {
	structs := collectStructFieldRanges(mod)
	if len(structs) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		vars := collectBodyVarTypes(fn.Body)
		inits := collectCompositeInitVars(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			be, ok := n.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhs, ok := be.X.(*syntax.Ident)
			if !ok {
				return true
			}
			targetInfo, ok := structs[vars[lhs.String()]]
			if !ok {
				return true
			}
			cl := compositeLiteralOf(be.Y, inits)
			if cl == nil {
				return true
			}
			srcType := ""
			if rid, ok := be.Y.(*syntax.Ident); ok {
				srcType = vars[rid.String()]
			}
			diags = append(diags, checkStructLiteralAgainst(cl, targetInfo, structs[srcType])...)
			return true
		})
	}
	return diags
}

// checkStructLiteralAgainst validates each field value of cl against
// targetInfo. For unions the literal's named alternative maps to the
// target's same-named field; for records/sets the source field name is
// resolved to its declared position via srcInfo and matched to the
// target field at that position (positional compatibility).
func checkStructLiteralAgainst(cl *syntax.CompositeLiteral, targetInfo structFieldInfo, srcInfo structFieldInfo) []Diagnostic {
	var diags []Diagnostic
	for idx, elem := range cl.List {
		if elem == nil {
			continue
		}
		name, val := splitFieldAssign(elem)
		var tgt *fieldConstraint
		if targetInfo.isUnion {
			if name == "" {
				continue
			}
			tgt = targetInfo.fieldByName(name)
		} else if name != "" {
			pos := srcInfo.indexOf(name)
			if pos < 0 {
				continue
			}
			tgt = targetInfo.fieldAt(pos)
		} else {
			tgt = targetInfo.fieldAt(idx)
		}
		if tgt == nil || !tgt.hasSpec {
			continue
		}
		num, fam, ok := numericLiteralValue(val)
		if !ok || !tgt.spec.familyMatches(fam) || tgt.spec.includes(num) {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "field-value-constraint-violation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"value %s violates target field %q's constraint %s (ETSI 6.3.2)",
				numericString(num), tgt.name, tgt.spec.describe()),
			Node: val,
			Span: syntax.SpanOf(val),
		})
	}
	return diags
}

// splitFieldAssign returns (name, value) for a `name := value`
// composite-literal entry, or ("", elem) for a positional entry.
func splitFieldAssign(elem syntax.Expr) (string, syntax.Expr) {
	if be, ok := elem.(*syntax.BinaryExpr); ok && be.Op != nil &&
		be.Op.Kind() == syntax.ASSIGN {
		if id, ok := be.X.(*syntax.Ident); ok && id != nil {
			return id.String(), be.Y
		}
	}
	return "", elem
}

func (s structFieldInfo) indexOf(name string) int {
	for i, f := range s.fields {
		if f.name == name {
			return i
		}
	}
	return -1
}

func (s structFieldInfo) fieldAt(i int) *fieldConstraint {
	if i < 0 || i >= len(s.fields) {
		return nil
	}
	return &s.fields[i]
}

func (s structFieldInfo) fieldByName(name string) *fieldConstraint {
	for i := range s.fields {
		if s.fields[i].name == name {
			return &s.fields[i]
		}
	}
	return nil
}

// collectStructFieldRanges builds, for every record/set/union type, an
// ordered list of its fields with any numeric range/value subtype
// constraint resolved into a valueSpec.
func collectStructFieldRanges(mod *syntax.Module) map[string]structFieldInfo {
	out := map[string]structFieldInfo{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil || st.KindTok == nil {
			continue
		}
		isUnion := st.KindTok.Kind() == syntax.UNION
		info := structFieldInfo{isUnion: isUnion}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			fc := fieldConstraint{name: f.Name.String()}
			if f.ValueConstraint != nil {
				if base := baseNumericFamily(f.Type); base != "" {
					if spec, ok := parseValueConstraint(f.ValueConstraint, base); ok {
						fc.spec = spec
						fc.hasSpec = true
					}
				}
			}
			info.fields = append(info.fields, fc)
		}
		out[st.Name.String()] = info
	}
	return out
}

// collectBodyVarTypes maps every `var T name` in body to its
// named type (plain identifier type references only). Body-scoped so
// same-named vars in sibling functions don't alias each other.
func collectBodyVarTypes(body syntax.Node) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		tn := identName(vd.Type)
		if tn == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = tn
		}
		return true
	})
	return out
}
