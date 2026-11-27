// definition_value_rules.go checks values that are known before a test
// component runs, or at compile time (ETSI ES 201 873-1 clauses 8.2.1,
// 10), and the types of values sent (6.3.4). The default value of a
// module parameter is resolved at the start of run time, so it calls no
// function that runs on a component; a constant used in a type
// definition is known at compile time, so its value is no function
// result:
//
//	const integer c_max := f_max();
//	type integer T (0 .. c_max); // error
//
// A value sent on a message port is of a type the port type lists, not
// merely of a compatible one.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
	"github.com/nokia/ntt/ttcn3/types"
)

func (a *Analyzer) checkDefinitionValueRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// called is the first user function called in e satisfying ok.
	called := func(e syntax.Node, ok func(*syntax.FuncDecl) bool) *syntax.FuncDecl {
		var found *syntax.FuncDecl
		syntax.Inspect(e, func(n syntax.Node) bool {
			if ce, isCall := n.(*syntax.CallExpr); isCall && found == nil {
				if f := m.funcDecl(ce.Fun); f != nil && ok(f) {
					found = f
				}
			}
			return found == nil
		})
		return found
	}
	// computed are the module constants whose values are function
	// results.
	computed := map[syntax.Node]string{}
	for _, d := range mod.Defs {
		vd, ok := d.Def.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil {
			continue
		}
		switch vd.KindTok.Kind() {
		case syntax.CONST:
			for _, x := range vd.Decls {
				if x == nil || x.Value == nil {
					continue
				}
				if f := called(x.Value, func(*syntax.FuncDecl) bool { return true }); f != nil {
					computed[x] = syntax.Name(f.Name)
					if len(vd.Decls) == 1 {
						computed[vd] = syntax.Name(f.Name)
					}
				}
			}
		case syntax.MODULEPAR:
			for _, x := range vd.Decls {
				if x == nil || x.Value == nil {
					continue
				}
				if f := called(x.Value, func(f *syntax.FuncDecl) bool { return f.RunsOn != nil }); f != nil {
					add(x.Value, "modulepar-component-dependent", fmt.Sprintf("the default value of %s calls %s, which runs on a component: it is not resolved at the start of run time (ETSI 8.2.1)", syntax.Name(x.Name), syntax.Name(f.Name)))
				}
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.SubTypeDecl, *syntax.StructTypeDecl:
			if len(computed) == 0 {
				return false
			}
			syntax.Inspect(x, func(c syntax.Node) bool {
				id, ok := c.(*syntax.Ident)
				if !ok {
					return true
				}
				for _, d := range tree.LookupWithDB(id, m.db) {
					if f, ok := computed[d.Node]; ok {
						add(id, "type-constant-not-compile-time", fmt.Sprintf("%s is the result of %s, not known at compile time: it is no constant of a type definition (ETSI 10)", id, f))
					}
				}
				return true
			})
			return false
		case *syntax.CallExpr:
			sel, ok := x.Fun.(*syntax.SelectorExpr)
			if !ok || syntax.Name(sel.Sel) != "send" || x.Args == nil || len(x.Args.List) == 0 {
				return true
			}
			id, ok := x.Args.List[0].(*syntax.Ident)
			if !ok {
				return true
			}
			t, ttree := m.declType(id)
			pt, ptree := m.portTypeOf(sel.X)
			if t == nil || pt == nil || pt.KindTok != nil && pt.KindTok.Kind() != syntax.MESSAGE {
				return true
			}
			want, ok := typeIdentity(ttree, m.db, t)
			if !ok {
				return true
			}
			for _, at := range pt.Attrs {
				pa, ok := at.(*syntax.PortAttribute)
				if !ok || pa.KindTok == nil || pa.KindTok.Kind() == syntax.IN || pa.KindTok.Kind() == syntax.ADDRESS {
					continue
				}
				for _, lt := range pa.Types {
					have, ok := typeIdentity(ptree, m.db, lt)
					if !ok || have == want {
						return true
					}
				}
			}
			add(id, "send-type-not-listed", fmt.Sprintf("%s is of type %s, which port type %s does not list (ETSI 6.3.4)", id, syntax.Name(t), syntax.Name(pt.Name)))
		}
		return true
	})
	return diags
}

// typeIdentity identifies the type t of tree: the declaration it names,
// or a predefined type's name; ok false when it is not known.
func typeIdentity(tree *ttcn3.Tree, db *ttcn3.DB, t syntax.Expr) (any, bool) {
	id, ok := t.(*syntax.Ident)
	if !ok || tree == nil {
		return nil, false
	}
	if _, ok := types.Predefined[id.String()]; ok {
		return id.String(), true
	}
	defs := tree.LookupWithDB(id, db)
	if len(defs) != 1 {
		return nil, false
	}
	return defs[0].Node, true
}
