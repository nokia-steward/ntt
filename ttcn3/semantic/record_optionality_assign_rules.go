// record_optionality_assign_rules.go enforces ETSI ES 201 873-1
// clause 6.3.2: two record/set types are assignment-compatible
// only when all corresponding fields have identical optionality.
// We flag the narrow shape `vT2 := vT1;` (and `var T2 vT2 := vT1`)
// where T1 and T2 are sibling record/set types with:
//
//   - the same field count;
//   - the same per-field base type names;
//   - at least one field with mismatched `optional` flag.
//
// Range and other constraint mismatches are intentionally left
// alone here; those need a richer type-compat pass.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRecordOptionalityAssignRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	records := collectRecordFieldTypes(mod)
	unions := collectUnionFieldTypes(mod)
	if len(records) == 0 && len(unions) == 0 {
		return nil
	}
	mismatch := func(a, b string) bool {
		if a == "" || b == "" || a == b {
			return false
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
		anyOptDiff := false
		for i := range fa {
			if fa[i].typeName == "" || fb[i].typeName == "" {
				return false
			}
			if fa[i].typeName != fb[i].typeName {
				return false
			}
			if fa[i].optional != fb[i].optional {
				anyOptDiff = true
			}
		}
		return anyOptDiff
	}
	unionMismatch := func(a, b string) bool {
		if a == "" || b == "" || a == b {
			return false
		}
		fa, ok := unions[a]
		if !ok {
			return false
		}
		fb, ok := unions[b]
		if !ok {
			return false
		}
		if len(fa) == 0 || len(fb) == 0 {
			return false
		}
		// Build name sets and compare.
		sa := map[string]bool{}
		for _, n := range fa {
			sa[n] = true
		}
		for _, n := range fb {
			if !sa[n] {
				return true
			}
		}
		sb := map[string]bool{}
		for _, n := range fb {
			sb[n] = true
		}
		for _, n := range fa {
			if !sb[n] {
				return true
			}
		}
		return false
	}
	funcParamTypes := collectFunctionParamTypes(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		varTypes := collectFuncBodyVarTypes(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			vd, ok := n.(*syntax.ValueDecl)
			if !ok || vd == nil || vd.KindTok == nil ||
				vd.KindTok.Kind() != syntax.VAR {
				return true
			}
			lhsType := identName(vd.Type)
			_, isRec := records[lhsType]
			_, isUnion := unions[lhsType]
			if !isRec && !isUnion {
				return true
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				rhsID, ok := dec.Value.(*syntax.Ident)
				if !ok || rhsID == nil {
					continue
				}
				rhsType := varTypes[rhsID.String()]
				if isRec && mismatch(lhsType, rhsType) {
					diags = append(diags, Diagnostic{
						Code:     "record-assign-optionality-mismatch",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"cannot assign value of type %q to variable of type %q: field optionality differs (ETSI 6.3.2)",
							rhsType, lhsType),
						Node: dec.Value,
						Span: syntax.SpanOf(dec.Value),
					})
				}
				if isUnion && unionMismatch(lhsType, rhsType) {
					diags = append(diags, Diagnostic{
						Code:     "union-assign-name-mismatch",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"cannot assign value of union type %q to variable of union type %q: alternative names differ (ETSI 6.3.2)",
							rhsType, lhsType),
						Node: dec.Value,
						Span: syntax.SpanOf(dec.Value),
					})
				}
			}
			return true
		})
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			be, ok := n.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil ||
				be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhsID, ok := be.X.(*syntax.Ident)
			if !ok || lhsID == nil {
				return true
			}
			rhsID, ok := be.Y.(*syntax.Ident)
			if !ok || rhsID == nil {
				return true
			}
			lhsType := varTypes[lhsID.String()]
			rhsType := varTypes[rhsID.String()]
			if mismatch(lhsType, rhsType) {
				diags = append(diags, Diagnostic{
					Code:     "record-assign-optionality-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"cannot assign value of type %q to variable of type %q: field optionality differs (ETSI 6.3.2)",
						rhsType, lhsType),
					Node: be,
					Span: syntax.SpanOf(be),
				})
			}
			if unionMismatch(lhsType, rhsType) {
				diags = append(diags, Diagnostic{
					Code:     "union-assign-name-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"cannot assign value of union type %q to variable of union type %q: alternative names differ (ETSI 6.3.2)",
						rhsType, lhsType),
					Node: be,
					Span: syntax.SpanOf(be),
				})
			}
			return true
		})
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil || ce.Args == nil {
				return true
			}
			fnID, ok := ce.Fun.(*syntax.Ident)
			if !ok || fnID == nil {
				return true
			}
			params, hasFn := funcParamTypes[fnID.String()]
			if !hasFn || len(params) != len(ce.Args.List) {
				return true
			}
			for i, arg := range ce.Args.List {
				argID, ok := arg.(*syntax.Ident)
				if !ok || argID == nil {
					continue
				}
				argType := varTypes[argID.String()]
				if !mismatch(params[i], argType) {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "param-pass-optionality-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"cannot pass value of type %q as actual parameter of type %q to function %q: field optionality differs (ETSI 5.4.2)",
						argType, params[i], fnID.String()),
					Node: arg,
					Span: syntax.SpanOf(arg),
				})
			}
			return true
		})
	}
	return diags
}

// collectUnionFieldTypes returns the ordered list of alternative
// names for each top-level union type in the module.
func collectUnionFieldTypes(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		std, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || std == nil || std.Name == nil || std.KindTok == nil {
			continue
		}
		if std.KindTok.Kind() != syntax.UNION {
			continue
		}
		var names []string
		for _, f := range std.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			names = append(names, f.Name.String())
		}
		out[std.Name.String()] = names
	}
	return out
}

// collectFunctionParamTypes returns the per-function ordered list
// of formal-parameter type names, used to validate actual-parameter
// type compatibility at call sites.
func collectFunctionParamTypes(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Name == nil || fn.Params == nil {
			continue
		}
		var pts []string
		for _, p := range fn.Params.List {
			if p == nil {
				continue
			}
			pts = append(pts, identName(p.Type))
		}
		out[fn.Name.String()] = pts
	}
	return out
}
