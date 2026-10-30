// interleave_goto_rules.go enforces ETSI ES 201 873-1 clause
// 20.4 restriction d on control-transfer statements inside
// an `interleave` branch:
//
//   - `for`, `while`, and `do-while` loops whose body
//     contains a reception statement are illegal;
//   - nested `alt` and `interleave` statements are illegal
//     because they would block on a competing event-set;
//   - `goto` to a label inside the interleave is illegal
//     whenever the surrounding branch body contains a
//     reception, since the branch then transfers control
//     across a blocking-event boundary, unless the goto and
//     its label lie in a statement block that contains no
//     reception. A jump out of the interleave is allowed,
//     conditional or not.
//
// We only inspect the immediate branch body; deeper nested
// scopes (e.g. inner functions) are intentionally skipped
// to keep false positives low.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkInterleaveGotoRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		alt, ok := n.(*syntax.AltStmt)
		if !ok || alt == nil || alt.Tok == nil ||
			alt.Tok.Kind() != syntax.INTERLEAVE || alt.Body == nil {
			return true
		}
		inside := collectLabelNames(alt.Body)
		for _, stmt := range alt.Body.Stmts {
			cc, ok := stmt.(*syntax.CommClause)
			if !ok || cc == nil || cc.Body == nil {
				continue
			}
			bodyHasReception := containsReceptionStmt(cc.Body)
			if bodyHasReception {
				for _, loop := range collectLoopStmts(cc.Body) {
					diags = append(diags, Diagnostic{
						Code:     "interleave-loop-in-reception-branch",
						Severity: SeverityError,
						Message:  "loops (`for`, `while`, `do-while`) inside an `interleave` branch are only allowed in reception-free branches (ETSI 20.4 restriction d a)",
						Node:     loop,
						Span:     syntax.SpanOf(loop),
					})
				}
				for _, g := range collectGotoStmts(cc.Body) {
					if !inside[syntax.Name(g.Label)] || jumpsWithinReceptionFreeBlock(cc.Body, g) {
						continue
					}
					diags = append(diags, Diagnostic{
						Code:     "interleave-goto-in-reception-branch",
						Severity: SeverityError,
						Message:  "`goto` inside an `interleave` branch is only allowed in reception-free branches or as a jump out of the interleave (ETSI 20.4 restriction d)",
						Node:     g,
						Span:     syntax.SpanOf(g),
					})
				}
			}
		}
		return true
	})
	return diags
}

// containsReceptionStmt reports whether the node transitively
// contains a port reception CallExpr (receive / trigger /
// getcall / getreply / catch / check / done / killed /
// timeout). The first hit short-circuits the walk.
func containsReceptionStmt(n syntax.Node) bool {
	found := false
	syntax.Inspect(n, func(node syntax.Node) bool {
		if found {
			return false
		}
		ce, ok := node.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok || op == nil {
			return true
		}
		switch op.String() {
		case "receive", "trigger", "getcall", "getreply",
			"catch", "check", "done", "killed", "timeout":
			found = true
			return false
		}
		return true
	})
	return found
}

// collectLoopStmts returns every ForStmt, ForRangeStmt,
// WhileStmt, and DoWhileStmt inside the node.
func collectLoopStmts(n syntax.Node) []syntax.Stmt {
	var out []syntax.Stmt
	syntax.Inspect(n, func(node syntax.Node) bool {
		switch v := node.(type) {
		case *syntax.ForStmt:
			if v != nil {
				out = append(out, v)
			}
		case *syntax.ForRangeStmt:
			if v != nil {
				out = append(out, v)
			}
		case *syntax.WhileStmt:
			if v != nil {
				out = append(out, v)
			}
		case *syntax.DoWhileStmt:
			if v != nil {
				out = append(out, v)
			}
		}
		return true
	})
	return out
}

// jumpsWithinReceptionFreeBlock reports whether goto g and its label lie
// in one statement block within body that contains no reception: the
// jumps 20.4 d allows inside an interleave.
func jumpsWithinReceptionFreeBlock(body *syntax.BlockStmt, g *syntax.BranchStmt) bool {
	label := syntax.Name(g.Label)
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		if found {
			return false
		}
		b, ok := n.(*syntax.BlockStmt)
		if !ok || b == nil {
			return true
		}
		hasGoto, hasLabel := false, false
		syntax.Inspect(b, func(m syntax.Node) bool {
			if bs, ok := m.(*syntax.BranchStmt); ok && bs != nil && bs.Tok != nil {
				hasGoto = hasGoto || bs == g
				hasLabel = hasLabel || (bs.Tok.Kind() == syntax.LABEL && syntax.Name(bs.Label) == label)
			}
			return true
		})
		if !hasGoto || !hasLabel {
			return false
		}
		if !containsReceptionStmt(b) {
			found = true
		}
		return true
	})
	return found
}

// collectLabelNames returns the name of every label declared within n.
func collectLabelNames(n syntax.Node) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(n, func(node syntax.Node) bool {
		if bs, ok := node.(*syntax.BranchStmt); ok && bs != nil && bs.Tok != nil && bs.Tok.Kind() == syntax.LABEL {
			out[syntax.Name(bs.Label)] = true
		}
		return true
	})
	return out
}

// collectGotoStmts returns every `goto` BranchStmt within the node.
// Labels are filtered out; we only care about goto-target
// statements for this rule.
func collectGotoStmts(n syntax.Node) []*syntax.BranchStmt {
	var out []*syntax.BranchStmt
	syntax.Inspect(n, func(node syntax.Node) bool {
		bs, ok := node.(*syntax.BranchStmt)
		if !ok || bs == nil || bs.Tok == nil {
			return true
		}
		if bs.Tok.Kind() == syntax.GOTO {
			out = append(out, bs)
		}
		return true
	})
	return out
}
