// execute_stmt_rules.go enforces ETSI ES 201 873-1 clause 26.1 type
// rules on the `execute(testcase, [timeout], [hostid])` statement:
//
//   - `timeout` (2nd argument) must be a float-typed expression and
//     must not be the literal `infinity` token (explicit infinite
//     guards are forbidden);
//   - `hostid` (3rd argument) must be a charstring-typed expression.
//
// Conservative: the rule only fires when the argument's declared
// type or literal kind is recognisable; references to opaque
// expressions (function calls, complex subscripts) stay silent.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkExecuteStmtRules(mod *syntax.Module) []Diagnostic {
	varTypes := collectModuleVarTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		fn, ok := ce.Fun.(*syntax.Ident)
		if !ok || fn == nil || fn.String() != "execute" {
			return true
		}
		if ce.Args == nil {
			return true
		}
		args := ce.Args.List
		// arg[0] is the testcase reference; type-checked elsewhere.
		if len(args) >= 2 {
			ty := classifyExecuteArg(args[1], varTypes)
			if ty == "infinity" {
				diags = append(diags, Diagnostic{
					Code:     "execute-explicit-infinity-timeout",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"execute(...) does not accept `infinity` as an explicit timeout (ETSI 26.1)"),
					Node: args[1],
					Span: syntax.SpanOf(args[1]),
				})
			} else if ty != "" && ty != "-" && ty != "float" {
				diags = append(diags, Diagnostic{
					Code:     "execute-timeout-not-float",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"execute(...) timeout argument must be float-typed (got %q) (ETSI 26.1)",
						ty),
					Node: args[1],
					Span: syntax.SpanOf(args[1]),
				})
			}
		}
		if len(args) >= 3 {
			ty := classifyExecuteArg(args[2], varTypes)
			if ty != "" && ty != "-" && ty != "charstring" {
				diags = append(diags, Diagnostic{
					Code:     "execute-hostid-not-charstring",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"execute(...) host-id argument must be charstring-typed (got %q) (ETSI 26.1)",
						ty),
					Node: args[2],
					Span: syntax.SpanOf(args[2]),
				})
			}
		}
		return true
	})
	return diags
}

// classifyExecuteArg returns a type label for an `execute` argument
// when it is a literal or a known variable reference; "" otherwise
// (skip-quietly contract). The `-` placeholder is reported verbatim
// so callers can short-circuit it.
func classifyExecuteArg(e syntax.Expr, varTypes map[string]string) string {
	if e == nil {
		return ""
	}
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		if v == nil || v.Tok == nil {
			return ""
		}
		switch v.Tok.Kind() {
		case syntax.FLOAT:
			return "float"
		case syntax.INT:
			return "integer"
		case syntax.STRING:
			return "charstring"
		case syntax.BSTRING:
			return "bitstring"
		case syntax.SUB:
			return "-"
		}
		switch v.Tok.String() {
		case "infinity":
			return "infinity"
		case "true", "false":
			return "boolean"
		}
	case *syntax.Ident:
		if v == nil {
			return ""
		}
		switch v.String() {
		case "infinity":
			return "infinity"
		case "true", "false":
			return "boolean"
		}
		if t, ok := varTypes[v.String()]; ok {
			return t
		}
	case *syntax.UnaryExpr:
		// `-1.0` parses as UnaryExpr{Op: -, X: 1.0}. Treat the
		// operand's type as the result type for our purposes.
		if v == nil {
			return ""
		}
		return classifyExecuteArg(v.X, varTypes)
	}
	return ""
}
