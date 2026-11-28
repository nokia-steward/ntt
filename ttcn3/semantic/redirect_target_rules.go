// redirect_target_rules.go checks where the redirections of a receiving
// operation store into (ETSI ES 201 873-1 clauses 15.3 a and 22.2.2): a
// variable or a field or element of one, not a template definition, which
// is not set with dot notation in communication events, and not a
// constant.
//
//	p.receive(mw_t) -> value mw_t.field1; // error
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRedirectTargetRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	var check func(e syntax.Expr)
	check = func(e syntax.Expr) {
		switch x := e.(type) {
		case *syntax.ParenExpr:
			for _, el := range x.List {
				check(el)
			}
			return
		case *syntax.BinaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.ASSIGN {
				check(x.X)
			}
			return
		}
		root := e
		for {
			if sel, ok := root.(*syntax.SelectorExpr); ok {
				root = sel.X
			} else if ix, ok := root.(*syntax.IndexExpr); ok {
				root = ix.X
			} else {
				break
			}
		}
		id, ok := root.(*syntax.Ident)
		if !ok {
			return
		}
		if what := m.unassignable(id); what != "" {
			diags = append(diags, Diagnostic{
				Code:     "redirect-target-not-variable",
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s is %s %s: a redirection stores into a variable (ETSI 15.3 a, 22.2.2)", sourceText(e), what, id),
				Node:     e,
				Span:     syntax.SpanOf(e),
			})
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		r, ok := n.(*syntax.RedirectExpr)
		if !ok {
			return true
		}
		for _, e := range r.Value {
			check(e)
		}
		for _, e := range r.Param {
			check(e)
		}
		for _, e := range []syntax.Expr{r.Verdict, r.Sender, r.Index, r.Timestamp} {
			if e != nil {
				check(e)
			}
		}
		return true
	})
	return diags
}

// unassignable names what id refers to, when that is a template
// definition or a constant, or is "".
func (m typeModel) unassignable(id *syntax.Ident) string {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return ""
	}
	if _, ok := defs[0].Node.(*syntax.TemplateDecl); ok {
		return "the template"
	}
	vd, _, _, ok := m.valueDeclOf(id)
	if !ok || vd.KindTok == nil {
		return ""
	}
	switch vd.KindTok.Kind() {
	case syntax.CONST:
		return "the constant"
	case syntax.TEMPLATE:
		return "the template"
	}
	return ""
}
