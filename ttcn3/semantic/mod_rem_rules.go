// mod_rem_rules.go enforces ETSI ES 201 873-1 clause 7.1.1:
//
//	the `mod` and `rem` arithmetic operators are defined on
//	integer operands only.
//
// We only fire when we can statically resolve both operands to a
// known non-integer scalar (predominantly `float`). Calls,
// composite expressions, and operands of unknown type are left
// to the runtime; this keeps the rule strictly additive.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkModRemRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
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
		varTypes := collectFuncBodyVarTypes(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			be, ok := n.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil {
				return true
			}
			op := be.Op.Kind()
			if op != syntax.MOD && op != syntax.REM {
				return true
			}
			for _, side := range []syntax.Expr{be.X, be.Y} {
				ty := resolveScalarType(side, varTypes)
				if ty == "" || ty == "integer" {
					continue
				}
				if !isFloatishType(ty) {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "mod-rem-non-integer",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`%s` operator requires integer operands; %q is %s (ETSI 7.1.1)",
						be.Op.String(), exprNameOrLiteral(side), ty),
					Node: side,
					Span: syntax.SpanOf(side),
				})
			}
			return true
		})
	}
	return diags
}

// resolveScalarType returns the declared type name of expr when it
// is a bare identifier referencing a local var/const, or a literal
// kind label otherwise. "" means "I don't know".
func resolveScalarType(expr syntax.Expr, varTypes map[string]string) string {
	switch e := expr.(type) {
	case *syntax.Ident:
		if e == nil {
			return ""
		}
		return varTypes[e.String()]
	case *syntax.ValueLiteral:
		if e == nil || e.Tok == nil {
			return ""
		}
		switch e.Tok.Kind() {
		case syntax.INT:
			return "integer"
		case syntax.FLOAT:
			return "float"
		case syntax.STRING:
			return "charstring"
		case syntax.BSTRING:
			return "bitstring"
		case syntax.TRUE, syntax.FALSE:
			return "boolean"
		}
	}
	return ""
}

// isFloatishType is a deliberately narrow whitelist of types that
// are demonstrably not integer. We do NOT flag the long tail of
// user-defined sub-types here; a future rule with a real type
// resolver can broaden this.
func isFloatishType(ty string) bool {
	switch ty {
	case "float", "charstring", "universal charstring",
		"bitstring", "octetstring", "hexstring",
		"boolean", "verdicttype", "anytype":
		return true
	}
	return false
}

func exprNameOrLiteral(expr syntax.Expr) string {
	switch e := expr.(type) {
	case *syntax.Ident:
		if e == nil {
			return "<expr>"
		}
		return e.String()
	case *syntax.ValueLiteral:
		if e == nil || e.Tok == nil {
			return "<literal>"
		}
		return e.Tok.String()
	}
	return "<expr>"
}
