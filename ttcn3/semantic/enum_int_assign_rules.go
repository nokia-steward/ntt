// enum_int_assign_rules.go enforces ETSI ES 201 873-1 clause 6.2.4
// "The user shall not directly use associated integer values [of
// enumerated values] but can access them and convert integer
// values into enumerated values by using the predefined functions
// enum2int and int2enum."
//
// We flag the cleanest, fully-local shape:
//
//	var integer  v_int := v_enum;       // v_enum is enum-typed
//	v_int := v_enum;                    // ditto, plain assignment
//	var EnumT    v_enum := v_int;       // v_int is integer-typed
//
// Identifier RHSs whose type we cannot resolve (function results,
// module-level vars, etc.) are left alone to avoid false positives.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkEnumIntAssignRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	enums := collectEnumTypeNames(mod)
	if len(enums) == 0 {
		return nil
	}
	isEnum := func(t string) bool { return t != "" && enums[t] }
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
			declTy := identName(vd.Type)
			for _, dec := range vd.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				rhsID, ok := dec.Value.(*syntax.Ident)
				if !ok || rhsID == nil {
					continue
				}
				rhsTy := varTypes[rhsID.String()]
				if declTy == "integer" && isEnum(rhsTy) {
					diags = append(diags, Diagnostic{
						Code:     "enum-to-integer-direct-assign",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"variable %q has type integer; assigning enum-typed %q directly is forbidden (use enum2int; ETSI 6.2.4)",
							dec.Name.String(), rhsID.String()),
						Node: dec.Value,
						Span: syntax.SpanOf(dec.Value),
					})
				}
				if isEnum(declTy) && rhsTy == "integer" {
					diags = append(diags, Diagnostic{
						Code:     "integer-to-enum-direct-assign",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"variable %q has enum type %q; assigning integer-typed %q directly is forbidden (use int2enum; ETSI 6.2.4)",
							dec.Name.String(), declTy, rhsID.String()),
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
			lhsTy := varTypes[lhsID.String()]
			rhsTy := varTypes[rhsID.String()]
			if lhsTy == "integer" && isEnum(rhsTy) {
				diags = append(diags, Diagnostic{
					Code:     "enum-to-integer-direct-assign",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"variable %q has type integer; assigning enum-typed %q directly is forbidden (use enum2int; ETSI 6.2.4)",
						lhsID.String(), rhsID.String()),
					Node: be,
					Span: syntax.SpanOf(be),
				})
			}
			if isEnum(lhsTy) && rhsTy == "integer" {
				diags = append(diags, Diagnostic{
					Code:     "integer-to-enum-direct-assign",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"variable %q has enum type %q; assigning integer-typed %q directly is forbidden (use int2enum; ETSI 6.2.4)",
						lhsID.String(), lhsTy, rhsID.String()),
					Node: be,
					Span: syntax.SpanOf(be),
				})
			}
			return true
		})
	}
	return diags
}
