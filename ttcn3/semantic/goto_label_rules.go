// goto_label_rules.go enforces ETSI ES 201 873-1 clause 19.8 "The
// goto statement" restrictions:
//
//   - A `goto L` must reference a label declared in the same
//     function / testcase / altstep body (jumping across function
//     boundaries is forbidden).
//   - The control-flow barriers (for / while / do-while / if /
//     select / alt / interleave) enclosing the label must also
//     enclose the goto, i.e. you cannot jump *into* a tighter
//     scope.
//
// We collect every `label L` and `goto L` BranchStmt inside each
// function body together with the chain of barrier statements that
// enclose them. A goto is well-formed iff some label with the same
// name exists whose barrier chain is a prefix of the goto's chain.
// All other shapes are diagnosed.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type gotoEntry struct {
	stmt     *syntax.BranchStmt
	barriers []syntax.Node
}

func (a *Analyzer) checkGotoLabelRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fd, ok := n.(*syntax.FuncDecl)
		if !ok || fd == nil || fd.Body == nil {
			return true
		}
		gotos, labels := collectGotosAndLabels(fd.Body)
		// ETSI 19.7: label names must be unique within the
		// same control-flow scope. Two labels sharing a name
		// AND a barrier chain are an error; sibling alt
		// branches / loop bodies are independent scopes.
		for name, cands := range labels {
			if len(cands) < 2 {
				continue
			}
			for i := 1; i < len(cands); i++ {
				for j := 0; j < i; j++ {
					if !sameBarrierChain(cands[i].barriers, cands[j].barriers) {
						continue
					}
					diags = append(diags, Diagnostic{
						Code:     "duplicate-label",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"duplicate label %q in the same control-flow scope (ETSI 19.7)",
							name),
						Node: cands[i].stmt,
						Span: syntax.SpanOf(cands[i].stmt),
					})
					break
				}
			}
		}
		for _, g := range gotos {
			if g.stmt.Label == nil {
				continue
			}
			name := g.stmt.Label.String()
			cands := labels[name]
			if len(cands) == 0 {
				diags = append(diags, Diagnostic{
					Code:     "goto-unknown-label",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"goto %q has no matching label in the enclosing function body (ETSI 19.8)",
						name),
					Node: g.stmt,
					Span: syntax.SpanOf(g.stmt),
				})
				continue
			}
			ok := false
			for _, l := range cands {
				if isBarrierPrefix(g.barriers, l.barriers) {
					ok = true
					break
				}
			}
			if !ok {
				diags = append(diags, Diagnostic{
					Code:     "goto-into-restricted-scope",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"goto %q jumps into a tighter control-flow scope (loop, alt, select, or if/else); not allowed (ETSI 19.8)",
						name),
					Node: g.stmt,
					Span: syntax.SpanOf(g.stmt),
				})
			}
		}
		return true
	})
	return diags
}

func collectGotosAndLabels(body *syntax.BlockStmt) ([]gotoEntry, map[string][]gotoEntry) {
	var gotos []gotoEntry
	labels := map[string][]gotoEntry{}
	var walk func(n syntax.Node, barriers []syntax.Node)
	walk = func(n syntax.Node, barriers []syntax.Node) {
		if n == nil {
			return
		}
		switch v := n.(type) {
		case *syntax.BranchStmt:
			if v == nil || v.Tok == nil {
				return
			}
			switch v.Tok.Kind() {
			case syntax.LABEL:
				if v.Label != nil {
					labels[v.Label.String()] = append(labels[v.Label.String()],
						gotoEntry{stmt: v, barriers: cloneBarriers(barriers)})
				}
			case syntax.GOTO:
				gotos = append(gotos, gotoEntry{
					stmt:     v,
					barriers: cloneBarriers(barriers),
				})
			}
			return
		case *syntax.ForStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			walk(v.Init, child)
			walk(v.Cond, child)
			walk(v.Post, child)
			walk(v.Body, child)
			return
		case *syntax.ForRangeStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			walk(v.Init, child)
			walk(v.Range, child)
			walk(v.Body, child)
			return
		case *syntax.WhileStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			walk(v.Cond, child)
			walk(v.Body, child)
			return
		case *syntax.DoWhileStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			walk(v.Cond, child)
			walk(v.Body, child)
			return
		case *syntax.IfStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			walk(v.Cond, child)
			walk(v.Then, child)
			walk(v.Else, child)
			return
		case *syntax.SelectStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			for _, cc := range v.Body {
				walk(cc, child)
			}
			return
		case *syntax.AltStmt:
			if v == nil {
				return
			}
			child := append(barriers, v)
			walk(v.Body, child)
			return
		}
		// Generic recurse via the Inspect machinery; we re-enter
		// `walk` to keep the barrier stack synchronised.
		n.Inspect(func(child syntax.Node) bool {
			if child == n {
				return true
			}
			walk(child, barriers)
			return false
		})
	}
	walk(body, nil)
	return gotos, labels
}

func cloneBarriers(b []syntax.Node) []syntax.Node {
	out := make([]syntax.Node, len(b))
	copy(out, b)
	return out
}

// sameBarrierChain reports whether two label/goto chains are
// pointwise identical. Used by the duplicate-label check to
// distinguish "same control-flow scope" (same barrier list)
// from "sibling scope" (e.g. two separate alt branches).
func sameBarrierChain(a, b []syntax.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// isBarrierPrefix reports whether the label is in the same or an
// outer control-flow scope than the goto - i.e. labelChain is a
// prefix of gotoChain. ETSI 19.8 forbids jumping *into* a tighter
// scope, so a longer or diverging label chain is rejected.
func isBarrierPrefix(gotoChain, labelChain []syntax.Node) bool {
	if len(labelChain) > len(gotoChain) {
		return false
	}
	for i := range labelChain {
		if labelChain[i] != gotoChain[i] {
			return false
		}
	}
	return true
}
