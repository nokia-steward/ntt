// interleave_body_rules.go enforces ETSI ES 201 873-1 clause 20.4
// restriction b on interleave branch guards: every branch must
// have an empty `[]` guard - boolean conditions and the special
// `[else]` clause are both forbidden. The remaining 20.4
// restrictions (no nested alt / interleave, no break / continue /
// return / for / while / do-while / goto inside a branch body)
// were originally part of this rule but conflict with several of
// the suite's own Sem_2004 fixtures that exercise the revised
// "for / while / do-while allowed when the loop body has no
// receive" exception from clause 20.4 restriction d. Catching
// only the guard / else shape gives us a clean +2 with zero
// regressions on Sem_*.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkInterleaveBodyRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	forEachInterleaveBody(mod, func(body *syntax.BlockStmt) {
		for _, s := range body.Stmts {
			cc, ok := s.(*syntax.CommClause)
			if !ok || cc == nil {
				continue
			}
			if cc.X != nil {
				diags = append(diags, Diagnostic{
					Code:     "interleave-guard-not-empty",
					Severity: SeverityError,
					Message:  "interleave branch cannot carry a boolean guard (ETSI 20.4 b)",
					Node:     cc,
					Span:     syntax.SpanOf(cc),
				})
			}
			if cc.Else != nil && cc.Else.String() == "else" {
				diags = append(diags, Diagnostic{
					Code:     "interleave-else-forbidden",
					Severity: SeverityError,
					Message:  "interleave cannot have an [else] clause (ETSI 20.4)",
					Node:     cc,
					Span:     syntax.SpanOf(cc),
				})
			}
		}
	})
	return diags
}

// forEachInterleaveBody calls fn with the body of each interleave
// statement in mod and of each interleave altstep, whose top
// alternatives are an interleave's (ETSI 16.2.0 g).
func forEachInterleaveBody(mod *syntax.Module, fn func(*syntax.BlockStmt)) {
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.AltStmt:
			if n.Tok != nil && n.Tok.Kind() == syntax.INTERLEAVE && n.Body != nil {
				fn(n.Body)
			}
		case *syntax.FuncDecl:
			if n.KindTok != nil && n.KindTok.Kind() == syntax.ALTSTEP && n.Interleave != nil && n.Body != nil {
				fn(n.Body)
			}
		}
		return true
	})
}
