// component_ref_rules.go checks component references and the ports of
// communication operations (ETSI ES 201 873-1 clauses 6.2.11, 21.1.1,
// 22.1.4.2, 22.2.1). A variable of a component type holds references to
// components of compatible types, which have all its definitions:
//
//	type component CA { var integer vc_a := 1 }
//	type component CB { var float vc_b := 1.0 }
//	var CA v_ref := CB.create; // error
//
// The component reference of a port in connect, disconnect, map or unmap
// is of a component type. A communication operation names the port it
// takes place at, and the value it sends is of a data type, not an
// object. The sender variable of a receiving operation is compatible
// with the component its from clause names.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkComponentRefRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	db := a.db()
	m := typeModel{tree: tree, db: db}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// created checks the reference v, of the component type typ declared
	// in ttree, to a component it creates.
	created := func(ttree *ttcn3.Tree, typ syntax.Expr, v syntax.Expr) {
		x := createdType(v)
		if x == nil || ttree == nil {
			return
		}
		want := componentDecl(ttree, db, typ)
		have := componentDecl(tree, db, x)
		if want != nil && have != nil && !componentCompatibleWith(tree, db, have, want, false) {
			add(v, "component-ref-incompatible", fmt.Sprintf("%s does not have all definitions of %s: its reference is no %s (ETSI 6.2.11, 6.3.3)", have.Name, want.Name, want.Name))
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			if x.KindTok == nil || x.KindTok.Kind() != syntax.VAR || x.TemplateRestriction != nil {
				return true
			}
			for _, d := range x.Decls {
				if d != nil && d.Value != nil && len(d.ArrayDef) == 0 {
					created(tree, x.Type, d.Value)
				}
			}
		case *syntax.ExprStmt:
			switch e := x.Expr.(type) {
			case *syntax.BinaryExpr:
				if e.Op != nil && e.Op.Kind() == syntax.ASSIGN {
					if id, ok := e.X.(*syntax.Ident); ok {
						if t, tt := m.declType(id); t != nil {
							created(tt, t, e.Y)
						}
					}
				}
			case *syntax.CallExpr:
				id, ok := e.Fun.(*syntax.Ident)
				if !ok || len(tree.LookupWithDB(id, db)) > 0 {
					return true
				}
				switch id.String() {
				case "send", "receive", "trigger", "check", "call", "getcall", "reply", "getreply", "raise", "catch":
					add(e, "port-op-without-port", fmt.Sprintf("%s names the port it takes place at, p.%s (ETSI 22.1.4.2 a)", id, id))
				}
			}
		case *syntax.RedirectExpr:
			// The sender variable is compatible with the component
			// the from clause names (22.2.2, 22.4 h).
			from, ok := x.X.(*syntax.BinaryExpr)
			sender, sok := x.Sender.(*syntax.Ident)
			if !ok || !sok || from.Op == nil || from.Op.Kind() != syntax.FROM {
				return true
			}
			src, ok := from.Y.(*syntax.Ident)
			if !ok {
				return true
			}
			st, stree := m.declType(src)
			vt, vtree := m.declType(sender)
			if st == nil || vt == nil {
				return true
			}
			have, want := componentDecl(stree, db, st), componentDecl(vtree, db, vt)
			if have != nil && want != nil && !componentCompatibleWith(tree, db, have, want, false) {
				add(sender, "sender-type-incompatible", fmt.Sprintf("the sender variable %s of type %s is not compatible with %s of type %s in the from clause (ETSI 22.4 h)", sender, want.Name, src, have.Name))
			}
		case *syntax.CallExpr:
			if sel, ok := x.Fun.(*syntax.SelectorExpr); ok && syntax.Name(sel.Sel) == "send" && x.Args != nil && len(x.Args.List) > 0 {
				if id, ok := x.Args.List[0].(*syntax.Ident); ok {
					if t, tt := m.declType(id); t != nil && isClassType(tt, db, t) {
						add(id, "send-non-data-type", fmt.Sprintf("%s refers to an object: what send sends is of a data type (ETSI 22.2.1 h)", id))
					}
				}
				return true
			}
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || x.Args == nil || len(tree.LookupWithDB(id, db)) > 0 {
				return true
			}
			switch id.String() {
			case "connect", "disconnect", "map", "unmap":
			default:
				return true
			}
			for _, arg := range x.Args.List {
				be, ok := arg.(*syntax.BinaryExpr)
				if !ok || be.Op == nil || be.Op.Kind() != syntax.COLON {
					continue
				}
				if r, ok := m.exprRoot(be.X); ok && (r.primitive() || r.structured()) {
					add(be.X, "port-ref-not-component", fmt.Sprintf("%s is a %s value, not a component reference (ETSI 6.2.11 b, 21.1.1)", sourceText(be.X), r.kind))
				}
			}
		}
		return true
	})
	return diags
}

// createdType is the component type T of a create operation v, T.create,
// or nil.
func createdType(v syntax.Expr) syntax.Expr {
	if ce, ok := v.(*syntax.CallExpr); ok {
		v = ce.Fun
	}
	sel, ok := v.(*syntax.SelectorExpr)
	if !ok || syntax.Name(sel.Sel) != "create" {
		return nil
	}
	if _, ok := sel.X.(*syntax.Ident); !ok {
		return nil
	}
	return sel.X
}

// isClassType reports whether the type t of tree is a class.
func isClassType(tree *ttcn3.Tree, db *ttcn3.DB, t syntax.Expr) bool {
	defs := tree.LookupWithDB(t, db)
	if len(defs) != 1 {
		return false
	}
	_, ok := defs[0].Node.(*syntax.ClassTypeDecl)
	return ok
}
