// send_value_type_rules.go checks that the type of a value sent on a
// message port is known (ETSI ES 201 873-1 clause 15.4). A literal
// value of a root type more than one of the port's outgoing types has,
// as through subtyping, is ambiguous, and the in-line template names its
// type:
//
//	type integer I1 (0..10);
//	type integer I2 (0..100);
//	type port P message { inout I1, I2 }
//	p.send(5);     // error
//	p.send(I1: 5); // the value is of type I1
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSendValueTypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || syntax.Name(sel.Sel) != "send" {
			return true
		}
		lit, ok := unparen(ce.Args.List[0]).(*syntax.ValueLiteral)
		if !ok {
			return true
		}
		v, ok := m.exprRoot(lit)
		if !ok || !v.primitive() {
			return true
		}
		pt, ptree := m.portTypeOf(sel.X)
		if pt == nil || pt.KindTok != nil && pt.KindTok.Kind() != syntax.MESSAGE {
			return true
		}
		var match []string
		for _, at := range pt.Attrs {
			pa, ok := at.(*syntax.PortAttribute)
			if !ok || pa.KindTok == nil || pa.KindTok.Kind() == syntax.IN || pa.KindTok.Kind() == syntax.ADDRESS {
				continue
			}
			for _, t := range pa.Types {
				if r, ok := m.typeRoot(ptree, t, 0); ok && r.kind == v.kind {
					match = append(match, syntax.Name(t))
				}
			}
		}
		if len(match) > 1 {
			diags = append(diags, Diagnostic{
				Code:     "send-value-type-ambiguous",
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s may be a value of %s, which the port sends: name its type, T: %s (ETSI 15.4)", sourceText(lit), strings.Join(match, " or "), sourceText(lit)),
				Node:     lit,
				Span:     syntax.SpanOf(lit),
			})
		}
		return true
	})
	return diags
}

// portTypeOf is the port type of the port e refers to, an element of a
// port array included, and the tree it is declared in.
func (m typeModel) portTypeOf(e syntax.Expr) (*syntax.PortTypeDecl, *ttcn3.Tree) {
	id, ok := rootRef(e).(*syntax.Ident)
	if !ok {
		return nil, nil
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return nil, nil
	}
	var t syntax.Expr
	switch d := defs[0].Node.(type) {
	case *syntax.Declarator:
		if vd, ok := defs[0].ParentOf(d).(*syntax.ValueDecl); ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.PORT {
			t = vd.Type
		}
	case *syntax.ValueDecl:
		if d.KindTok != nil && d.KindTok.Kind() == syntax.PORT {
			t = d.Type
		}
	case *syntax.FormalPar:
		t = d.Type
	}
	if t == nil {
		return nil, nil
	}
	pdefs := defs[0].Tree.LookupWithDB(t, m.db)
	if len(pdefs) != 1 {
		return nil, nil
	}
	if pt, ok := pdefs[0].Node.(*syntax.PortTypeDecl); ok {
		return pt, pdefs[0].Tree
	}
	return nil, nil
}
