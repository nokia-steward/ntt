// expr_operand_rules.go enforces ETSI ES 201 873-1 clause 7
// restrictions on operator operands:
//
//   - A function declared without a `return T` clause cannot
//     appear as an operand of a binary expression: the call
//     yields no value (clause 7).
//
// The check is intentionally narrow: it only fires on a direct
// CallExpr operand of a BinaryExpr whose callee Ident resolves to
// a module-local FUNCTION (not testcase / altstep) without a
// return spec. External functions, signatures, function-pointer
// expressions, and nested-expression callsites are left alone.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkExprOperandRules(mod *syntax.Module) []Diagnostic {
	noReturnFns := collectNoReturnFunctionNames(mod)
	templateVars := collectTemplateVarNames(mod)
	if len(noReturnFns) == 0 && len(templateVars) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil {
			return true
		}
		if !isValueExprOperator(be.Op.String()) {
			return true
		}
		for _, side := range []syntax.Expr{be.X, be.Y} {
			if ce, ok := side.(*syntax.CallExpr); ok && ce != nil {
				if id, ok := ce.Fun.(*syntax.Ident); ok && id != nil && noReturnFns[id.String()] {
					diags = append(diags, Diagnostic{
						Code:     "expr-operand-no-return-function",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"function %q has no `return` clause and cannot be an operand of `%s` (ETSI 7)",
							id.String(), be.Op.String()),
						Node: ce,
						Span: syntax.SpanOf(ce),
					})
					continue
				}
			}
			if id, ok := side.(*syntax.Ident); ok && id != nil && templateVars[id.String()] {
				diags = append(diags, Diagnostic{
					Code:     "expr-operand-template-var",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"template variable %q cannot be an operand of `%s`; expressions require value operands (ETSI 7)",
						id.String(), be.Op.String()),
					Node: id,
					Span: syntax.SpanOf(id),
				})
			}
		}
		return true
	})
	return diags
}

// collectTemplateVarNames returns every `var template T name` / `var
// template(restriction) T name` declared at module / body scope.
// Plain `template T name := ...` declarations (the TemplateDecl
// shape) are NOT included because they are conventionally used as
// matching shapes against received messages and many existing tests
// reference them as `valueof(t)` etc.
func collectTemplateVarNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		if vd.KindTok == nil || vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction == nil {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			out[d.Name.String()] = true
		}
		return true
	})
	return out
}

// isValueExprOperator reports whether the operator requires both
// operands to yield a value (arithmetic, ordered comparison,
// shift, concatenation). Equality and logical operators stay out
// of scope: `==` / `!=` are allowed against `null` per 7.1.3 and
// `and` / `or` / `xor` are boolean - the predominant misuse is
// arithmetic with a non-returning function.
func isValueExprOperator(op string) bool {
	switch op {
	case "+", "-", "*", "/", "mod", "rem",
		"<", "<=", ">", ">=",
		"<<", ">>", "<@", "@>",
		"&":
		return true
	}
	return false
}

// collectNoReturnFunctionNames returns every module-local FUNCTION
// declaration (not testcase / altstep) lacking a return spec.
// External functions are skipped because the parser still gives
// them a FuncDecl shell - assuming "no return" for those would be
// wrong without a real signature DB.
func collectNoReturnFunctionNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Name == nil {
			continue
		}
		if fn.External != nil {
			continue
		}
		if fn.KindTok == nil || fn.KindTok.Kind() != syntax.FUNCTION {
			continue
		}
		if fn.Return == nil {
			out[fn.Name.String()] = true
		}
	}
	return out
}
