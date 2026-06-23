// call_stmt_rules.go enforces ETSI ES 201 873-1 clause 22.3.1
// restriction h: the response-and-exception-handling block of a
// `p.call(...) { ... }` statement cannot contain
//
//   - an `[else]` clause, or
//   - an altstep invocation as a top-level alternative.
//
// We walk every CallStmt and inspect its Body's communication
// clauses for either shape.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkCallStmtBodyRules(mod *syntax.Module) []Diagnostic {
	altsteps := collectAltstepNames(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		cs, ok := n.(*syntax.CallStmt)
		if !ok || cs == nil || cs.Body == nil {
			return true
		}
		for _, stmt := range cs.Body.Stmts {
			cc, ok := stmt.(*syntax.CommClause)
			if !ok || cc == nil {
				continue
			}
			if cc.Else != nil {
				diags = append(diags, Diagnostic{
					Code:     "call-block-else-clause",
					Severity: SeverityError,
					Message:  "call(...) response-handling block cannot contain an [else] clause (ETSI 22.3.1 h)",
					Node:     cc,
					Span:     syntax.SpanOf(cc),
				})
				continue
			}
			if name := altstepCallName(cc.Comm); name != "" && altsteps[name] {
				diags = append(diags, Diagnostic{
					Code:     "call-block-altstep-invocation",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"call(...) response-handling block cannot invoke altstep %q (ETSI 22.3.1 h)",
						name),
					Node: cc,
					Span: syntax.SpanOf(cc),
				})
			}
		}
		return true
	})
	return diags
}

// collectAltstepNames returns the set of module-level altstep
// names declared as FuncDecl with KindTok==ALTSTEP.
func collectAltstepNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fd, ok := n.(*syntax.FuncDecl)
		if !ok || fd == nil || fd.Name == nil || fd.KindTok == nil {
			return true
		}
		if fd.KindTok.Kind() == syntax.ALTSTEP {
			out[fd.Name.String()] = true
		}
		return true
	})
	return out
}

// altstepCallName returns the bare callee name when the comm
// statement is an ExprStmt holding a CallExpr to a plain Ident
// (e.g. `a_handleReply()`). Anything else (port operation,
// receiver-style call, BinaryExpr) returns "".
func altstepCallName(s syntax.Stmt) string {
	es, ok := s.(*syntax.ExprStmt)
	if !ok || es == nil {
		return ""
	}
	ce, ok := es.Expr.(*syntax.CallExpr)
	if !ok || ce == nil {
		return ""
	}
	return identName(ce.Fun)
}
