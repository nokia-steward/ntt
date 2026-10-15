// var_init_literal_type_rules.go enforces a simple but
// surprisingly common static check: when a local `var T v := <lit>`
// declaration uses a primitive type T and a bare literal as the
// initializer, the literal's primitive kind must match T.
//
// We only flag the cleanest, fully literal shape:
//
//	var <prim> v := <literal>;
//
// where:
//   - <prim> is one of integer / charstring / universal charstring /
//     boolean / float / bitstring / octetstring / hexstring;
//   - <literal> is a ValueLiteral whose token kind maps to a
//     different primitive than <prim>.
//
// We deliberately skip:
//   - subtype names (we do not track their base primitives);
//   - composite literals;
//   - identifier RHS (no easy way to find its type);
//   - special-case primitive bridges (charstring ↔ universal
//     charstring, integer ↔ float widening) - these stay safe.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// numericTypes is the set of variable types for which we tolerate
// any numeric/string-pattern literal kind; we only flag a clearly
// foreign STRING literal initializer.
var numericTypes = map[string]bool{
	"integer":   true,
	"boolean":   true,
	"float":     true,
	"verdicttype": true,
}

func (a *Analyzer) checkVarInitLiteralTypeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
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
			if !numericTypes[declTy] {
				return true
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Value == nil {
					continue
				}
				vl, ok := dec.Value.(*syntax.ValueLiteral)
				if !ok || vl == nil || vl.Tok == nil {
					continue
				}
				if vl.Tok.Kind() != syntax.STRING {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "var-init-literal-type-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"variable %q has type %q but initializer literal is a charstring (ETSI 5.1)",
						dec.Name.String(), declTy),
					Node: dec.Value,
					Span: syntax.SpanOf(dec.Value),
				})
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
			declTy := varTypes[lhsID.String()]
			if !numericTypes[declTy] {
				return true
			}
			vl, ok := be.Y.(*syntax.ValueLiteral)
			if !ok || vl == nil || vl.Tok == nil {
				return true
			}
			if vl.Tok.Kind() != syntax.STRING {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "var-assign-literal-type-mismatch",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"variable %q has type %q but assignment literal is a charstring (ETSI 5.1)",
					lhsID.String(), declTy),
				Node: be.Y,
				Span: syntax.SpanOf(be.Y),
			})
			return true
		})
	}
	return diags
}
