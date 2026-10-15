package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkReturnConsistency catches the cheapest, highest-signal class of
// return-type bugs: `return <expr>` inside a function with no return type,
// and `return;` inside one that declares a return type. Path-sensitive
// checks ("every branch ends with return") need real control-flow analysis
// and live in the runtime/IR layers instead.
func (a *Analyzer) checkReturnConsistency(mod *syntax.Module) []Diagnostic {
	var out []Diagnostic
	mod.Inspect(func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		hasReturn := fn.Return != nil && fn.Return.Type != nil

		if fn.Body == nil {
			return true
		}
		fn.Body.Inspect(func(stmt syntax.Node) bool {
			// Don't descend into nested behaviour declarations: their
			// returns are scoped to their own signatures.
			if nested, ok := stmt.(*syntax.FuncDecl); ok && nested != fn {
				return false
			}
			ret, ok := stmt.(*syntax.ReturnStmt)
			if !ok {
				return true
			}
			switch {
			case hasReturn && ret.Result == nil:
				out = append(out, Diagnostic{
					Code:     "return.missing-value",
					Severity: SeverityError,
					Message:  "function declares a return type but `return` carries no value",
					Node:     ret,
					Span:     syntax.SpanOf(ret),
				})
			case !hasReturn && ret.Result != nil:
				out = append(out, Diagnostic{
					Code:     "return.unexpected-value",
					Severity: SeverityError,
					Message:  "function has no return type but `return` carries a value",
					Node:     ret,
					Span:     syntax.SpanOf(ret),
				})
			}
			return true
		})
		return true
	})
	return out
}
