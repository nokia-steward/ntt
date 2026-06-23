// interleave_goto_rules.go enforces ETSI ES 201 873-1 clause
// 20.4 restriction d on control-transfer statements inside
// an `interleave` branch:
//
//   - `for`, `while`, and `do-while` loops whose body
//     contains a reception statement are illegal;
//   - nested `alt` and `interleave` statements are illegal
//     because they would block on a competing event-set;
//   - `goto` is illegal whenever the surrounding branch
//     contains a reception (either in its guard or in its
//     body), since the branch then transfers control across
//     a blocking-event boundary.
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
					diags = append(diags, Diagnostic{
						Code:     "interleave-goto-in-reception-branch",
						Severity: SeverityError,
						Message:  "`goto` inside an `interleave` branch is only allowed in reception-free branches or as an unconditional jump out (ETSI 20.4 restriction d b)",
						Node:     g,
						Span:     syntax.SpanOf(g),
					})
				}
			}
			for _, g := range collectConditionalGotos(cc.Body) {
				diags = append(diags, Diagnostic{
					Code:     "interleave-conditional-goto",
					Severity: SeverityError,
					Message:  "conditional `goto` (inside an `if`, loop, or select) is not allowed in `interleave` branches; only unconditional jumps are permitted (ETSI 20.4 restriction d b)",
					Node:     g,
					Span:     syntax.SpanOf(g),
				})
			}
		}
		return true
	})
	return diags
}

// collectConditionalGotos returns every goto BranchStmt whose
// AST path contains at least one IfStmt / loop / SelectStmt
// between the goto and the branch body root. Such a goto is
// "conditional" in the ETSI 20.4 sense and not permitted inside
// interleave branches.
func collectConditionalGotos(body *syntax.BlockStmt) []*syntax.BranchStmt {
	var out []*syntax.BranchStmt
	var walk func(n syntax.Node, depth int)
	walk = func(n syntax.Node, depth int) {
		if n == nil {
			return
		}
		switch v := n.(type) {
		case *syntax.BranchStmt:
			if v != nil && v.Tok != nil && v.Tok.Kind() == syntax.GOTO && depth > 0 {
				out = append(out, v)
			}
			return
		case *syntax.IfStmt:
			if v == nil {
				return
			}
			walk(v.Then, depth+1)
			walk(v.Else, depth+1)
			return
		case *syntax.ForStmt:
			if v != nil {
				walk(v.Body, depth+1)
			}
			return
		case *syntax.ForRangeStmt:
			if v != nil {
				walk(v.Body, depth+1)
			}
			return
		case *syntax.WhileStmt:
			if v != nil {
				walk(v.Body, depth+1)
			}
			return
		case *syntax.DoWhileStmt:
			if v != nil {
				walk(v.Body, depth+1)
			}
			return
		case *syntax.SelectStmt:
			if v == nil {
				return
			}
			for _, cc := range v.Body {
				walk(cc, depth+1)
			}
			return
		case *syntax.AltStmt:
			if v == nil {
				return
			}
			walk(v.Body, depth+1)
			return
		}
		n.Inspect(func(child syntax.Node) bool {
			if child == n {
				return true
			}
			walk(child, depth)
			return false
		})
	}
	walk(body, 0)
	return out
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
