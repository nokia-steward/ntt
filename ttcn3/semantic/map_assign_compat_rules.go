// map_assign_compat_rules.go enforces ETSI ES 201 873-1 clause
// 6.3.8 "compatibility of map types":
//
//	map types are compatible only with other map types whose
//	from-type and to-type are pairwise compatible.
//
// We only address the two narrow shapes the conformance suite
// exercises:
//
//   - `var T x := mapVar;` where `T` is not a map type at all
//     (a record / set / record-of / set-of, an array, a
//     scalar, etc.). NegSem_060308_..._001.
//   - `var M2 x := mapVar;` where `M1` (the type of `mapVar`)
//     and `M2` differ in from-type or to-type. NegSem_060308_..._002
//     and 003.
//
// Map-to-map type compatibility is delegated to the existing
// `map` collector: two maps are considered compatible iff their
// from-type and to-type names match literally. A future
// structural-equivalence pass can broaden this.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// rawMapSpec retains the raw from/to type identifier names from
// the original `type map from X to Y M` declaration. We use raw
// names (not the basic-type families) so we can flag a TMap1 vs
// TMap2 mismatch even when both froms and tos are user-defined
// record types.
type rawMapSpec struct {
	from string
	to   string
}

type recordFieldFull struct {
	name     string
	typeName string
	optional bool
}

func collectRecordFieldTypes(mod *syntax.Module) map[string][]recordFieldFull {
	out := map[string][]recordFieldFull{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		std, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || std == nil || std.Name == nil || std.KindTok == nil {
			continue
		}
		if std.KindTok.Kind() != syntax.RECORD &&
			std.KindTok.Kind() != syntax.SET {
			continue
		}
		var fs []recordFieldFull
		for _, f := range std.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			tn := typeSpecToName(f.Type)
			fs = append(fs, recordFieldFull{
				name:     f.Name.String(),
				typeName: tn,
				optional: f.Optional != nil && f.Optional.Kind() == syntax.OPTIONAL,
			})
		}
		out[std.Name.String()] = fs
	}
	return out
}

func recordEquivalent(a, b string, records map[string][]recordFieldFull) bool {
	if a == b && a != "" {
		return true
	}
	fa, ok := records[a]
	if !ok {
		return false
	}
	fb, ok := records[b]
	if !ok {
		return false
	}
	if len(fa) != len(fb) || len(fa) == 0 {
		return false
	}
	for i := range fa {
		if fa[i].typeName == "" || fb[i].typeName == "" {
			return false
		}
		if fa[i].typeName != fb[i].typeName {
			return false
		}
		if fa[i].optional != fb[i].optional {
			return false
		}
	}
	return true
}

func collectRawMapSpecs(mod *syntax.Module) map[string]rawMapSpec {
	out := map[string]rawMapSpec{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.MapTypeDecl:
			if x == nil || x.Name == nil || x.Spec == nil {
				continue
			}
			out[x.Name.String()] = rawMapSpec{
				from: typeSpecToName(x.Spec.FromType),
				to:   typeSpecToName(x.Spec.ToType),
			}
		case *syntax.SubTypeDecl:
			if x.Field == nil || x.Field.Name == nil {
				continue
			}
			ms, ok := x.Field.Type.(*syntax.MapSpec)
			if !ok || ms == nil {
				continue
			}
			out[x.Field.Name.String()] = rawMapSpec{
				from: typeSpecToName(ms.FromType),
				to:   typeSpecToName(ms.ToType),
			}
		}
	}
	return out
}

func (a *Analyzer) checkMapAssignCompatRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	maps := collectRawMapSpecs(mod)
	if len(maps) == 0 {
		return nil
	}
	records := collectRecordFieldTypes(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectFuncBodyVarTypes(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			vd, ok := n.(*syntax.ValueDecl)
			if !ok || vd == nil || vd.KindTok == nil ||
				vd.KindTok.Kind() != syntax.VAR {
				return true
			}
			lhsTypeName := identName(vd.Type)
			for _, dec := range vd.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				rhsID, ok := dec.Value.(*syntax.Ident)
				if !ok || rhsID == nil {
					continue
				}
				rhsType := varTypes[rhsID.String()]
				rhsMap, rhsIsMap := maps[rhsType]
				if !rhsIsMap {
					continue
				}
				if lhsTypeName == "" {
					continue
				}
				if lhsTypeName == rhsType {
					continue
				}
				lhsMap, lhsIsMap := maps[lhsTypeName]
				if !lhsIsMap {
					diags = append(diags, Diagnostic{
						Code:     "map-assign-non-map",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"cannot assign map %q to non-map type %q (ETSI 6.3.8)",
							rhsID.String(), lhsTypeName),
						Node: dec.Value,
						Span: syntax.SpanOf(dec.Value),
					})
					continue
				}
				if lhsMap.from != rhsMap.from || lhsMap.to != rhsMap.to {
					if recordEquivalent(lhsMap.from, rhsMap.from, records) &&
						recordEquivalent(lhsMap.to, rhsMap.to, records) {
						continue
					}
					diags = append(diags, Diagnostic{
						Code:     "map-assign-incompatible-map",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"cannot assign map %q (from %s to %s) to map type %q (from %s to %s) (ETSI 6.3.8)",
							rhsID.String(), rhsMap.from, rhsMap.to,
							lhsTypeName, lhsMap.from, lhsMap.to),
						Node: dec.Value,
						Span: syntax.SpanOf(dec.Value),
					})
				}
			}
			return true
		})
	}
	return diags
}
