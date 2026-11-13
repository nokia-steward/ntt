// range_loop_v5_rules.go checks the range-based loops of a module written
// in the next major release of the core language (syntax.V5). A loop whose
// iteration variable is declared outside it iterates a range of which
// every element is initialised (the task forces' decision, Berlin 2026);
// V4 lets it take an uninitialised one, which leaves the variable unbound
// (19.4.2). The analyzer tells only a literal range, whose `-` it sees.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRangeLoopV5Rules(mod *syntax.Module) []Diagnostic {
	if mod.Edition() != syntax.V5 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		loop, ok := n.(*syntax.ForRangeStmt)
		if !ok || loop == nil {
			return true
		}
		if _, outside := loop.Init.(*syntax.ExprStmt); !outside {
			return true
		}
		lit, ok := loop.Range.(*syntax.CompositeLiteral)
		if !ok || lit == nil {
			return true
		}
		for _, e := range lit.List {
			if isDashLiteral(e) {
				diags = append(diags, Diagnostic{
					Code:     "range-loop-uninitialised-element",
					Severity: SeverityError,
					Message:  "the range of a loop whose iteration variable is declared outside it has an uninitialised element",
					Node:     e,
					Span:     syntax.SpanOf(e),
				})
			}
		}
		return true
	})
	return diags
}
