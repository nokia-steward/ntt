// function_clause_rules.go enforces ETSI ES 201 873-1 clause
// 16.1 restrictions on the optional clauses attached to a
// `function` declaration:
//
//   - a function carrying a `system` clause must also carry
//     a `runs on` clause. A function without a `runs on`
//     spec is invoked from a context that does not own a
//     component (e.g. module control), and therefore cannot
//     produce a system component instance for the SUT.
//
// This is a narrow, purely syntactic shape check; cohorts
// NegSem_1601_toplevel_007 and any future tests that put a
// `system` spec on a plain function rely on it being
// rejected, while a function with both clauses
// (Sem_060210_ReuseofComponentTypes_002) is fully legal.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkFunctionClauseRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fd, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fd == nil || fd.KindTok == nil {
			continue
		}
		if fd.KindTok.Kind() != syntax.FUNCTION {
			continue
		}
		if fd.System != nil && fd.RunsOn == nil {
			diags = append(diags, Diagnostic{
				Code:     "function-system-without-runs-on",
				Severity: SeverityError,
				Message:  "a `function` with a `system` clause must also declare a `runs on` clause (ETSI 16.1)",
				Node:     fd.System,
				Span:     syntax.SpanOf(fd.System),
			})
		}
	}
	return diags
}
