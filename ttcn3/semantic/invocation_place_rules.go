// invocation_place_rules.go checks where behaviour is invoked (ETSI ES
// 201 873-1 clauses 16.1.0 b and 16.2.0): a function started on a test
// component has a runs on clause, and an altstep with the @control
// modifier runs on the control component only, from the control part or
// from control functions and altsteps.
//
//	v_ptc.start(f_noRunsOn()); // error
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkInvocationPlaceRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	behaviour := func(e syntax.Expr) *syntax.FuncDecl {
		id, ok := e.(*syntax.Ident)
		if !ok {
			return nil
		}
		defs := tree.LookupWithDB(id, m.db)
		if len(defs) != 1 {
			return nil
		}
		fd, _ := defs[0].Node.(*syntax.FuncDecl)
		return fd
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fd, ok := n.(*syntax.FuncDecl)
		if !ok || fd.Body == nil || fd.KindTok == nil {
			return true
		}
		control := isControlFunction(fd) || fd.KindTok.Kind() == syntax.ALTSTEP && fd.Modif != nil && fd.Modif.String() == "@control"
		syntax.Inspect(fd.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			if g := behaviour(ce.Fun); g != nil && !control && g.KindTok != nil && g.KindTok.Kind() == syntax.ALTSTEP && g.Modif != nil && g.Modif.String() == "@control" {
				add(ce, "control-altstep-in-test-behaviour", fmt.Sprintf("altstep %s is @control: it runs on the control component, not in %s %s (ETSI 16.2.0)",
					syntax.Name(g.Name), fd.KindTok.String(), syntax.Name(fd.Name)))
			}
			sel, ok := ce.Fun.(*syntax.SelectorExpr)
			if !ok || syntax.Name(sel.Sel) != "start" || ce.Args == nil || len(ce.Args.List) != 1 || !m.isComponentRef(sel.X) {
				return true
			}
			inner, ok := ce.Args.List[0].(*syntax.CallExpr)
			if !ok {
				return true
			}
			if g := behaviour(inner.Fun); g != nil && g.KindTok != nil && g.KindTok.Kind() == syntax.FUNCTION && g.RunsOn == nil {
				add(inner, "start-without-runs-on", fmt.Sprintf("function %s is started on a test component, so it has a runs on clause (ETSI 16.1.0 b)", syntax.Name(g.Name)))
			}
			return true
		})
		return false
	})
	return diags
}

// isComponentRef reports whether e is a variable, constant or parameter
// of a component type.
func (m typeModel) isComponentRef(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	if !ok {
		return false
	}
	t, tree := m.declType(id)
	if t == nil {
		return false
	}
	tid, ok := t.(*syntax.Ident)
	if !ok {
		return false
	}
	defs := tree.LookupWithDB(tid, m.db)
	if len(defs) != 1 {
		return false
	}
	_, ok = defs[0].Node.(*syntax.ComponentTypeDecl)
	return ok
}
