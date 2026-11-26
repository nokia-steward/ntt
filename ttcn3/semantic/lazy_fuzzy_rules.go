// lazy_fuzzy_rules.go checks lazy and fuzzy variables and parameters
// (ETSI ES 201 873-1 clauses 11.2 i, j, 5.4.1.1 n and 5.4.1.2 j):
//
//   - the initialisation or assignment of a lazy or fuzzy variable calls
//     no function with out or inout parameters;
//
//   - one used in a deterministic context, the template of a receiving
//     operation, an alt guard or a global template that is not fuzzy, is
//     declared @deterministic;
//
//   - and a @deterministic one is initialised with no operation 16.1.4
//     forbids in such a context, the predefined rnd among them.
//
//     var template @fuzzy integer v_t := 1;
//     p.receive(v_t); // error: v_t is not @deterministic
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkLazyFuzzyRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	callee := func(e syntax.Expr) *syntax.FuncDecl {
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
	// initialiser checks the value e given to the lazy or fuzzy name.
	initialiser := func(name string, e syntax.Expr, deterministic bool) {
		syntax.Inspect(e, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			if deterministic && syntax.Name(ce.Fun) == "rnd" {
				add(ce, "deterministic-init-side-effect", fmt.Sprintf("%s is @deterministic: the predefined rnd is no part of its value (ETSI 11.2 j, 16.1.4)", name))
				return true
			}
			fd := callee(ce.Fun)
			if fd == nil {
				return true
			}
			if fd.Params != nil {
				for _, p := range fd.Params.List {
					if p != nil && p.Direction != nil && (p.Direction.Kind() == syntax.OUT || p.Direction.Kind() == syntax.INOUT) {
						add(ce, "lazy-fuzzy-init-out-param", fmt.Sprintf("%s is lazy or fuzzy: %s, with %s parameter %s, is not called in its value (ETSI 11.2 i)",
							name, syntax.Name(fd.Name), p.Direction.String(), syntax.Name(p.Name)))
						return true
					}
				}
			}
			if deterministic && fd.Body != nil {
				if reason := bodyDeterministicViolation(fd.Body); reason != "" {
					add(ce, "deterministic-init-side-effect", fmt.Sprintf("%s is @deterministic: %s, which %s, is no part of its value (ETSI 11.2 j, 16.1.4)", name, syntax.Name(fd.Name), reason))
				}
			}
			return true
		})
	}
	// deterministicUses reports the lazy and fuzzy variables and
	// parameters e refers to that are not @deterministic.
	var deterministicUses func(e syntax.Node, where string)
	deterministicUses = func(e syntax.Node, where string) {
		syntax.Inspect(e, func(n syntax.Node) bool {
			// What a redirection stores into is not used.
			if r, ok := n.(*syntax.RedirectExpr); ok {
				deterministicUses(r.X, where)
				return false
			}
			id, ok := n.(*syntax.Ident)
			if !ok {
				return true
			}
			if modif := m.nonDeterministicModif(id); modif != "" {
				add(id, "lazy-fuzzy-not-deterministic", fmt.Sprintf("%s is %s and used in %s, a deterministic context, so it is declared @deterministic (ETSI 11.2 j, 5.4.1 n, j)", id, modif, where))
			}
			return true
		})
	}
	for _, d := range mod.Defs {
		if td, ok := d.Def.(*syntax.TemplateDecl); ok && td.Value != nil && (td.Modif == nil || td.Modif.String() != "@fuzzy") {
			deterministicUses(td.Value, "the global template "+syntax.Name(td.Name))
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			if x.Modif == nil || x.Modif.String() != "@lazy" && x.Modif.String() != "@fuzzy" {
				return true
			}
			for _, dc := range x.Decls {
				if dc != nil && dc.Value != nil {
					initialiser(syntax.Name(dc.Name), dc.Value, x.Deterministic != nil)
				}
			}
		case *syntax.ExprStmt:
			be, ok := x.Expr.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			id, ok := be.X.(*syntax.Ident)
			if !ok {
				return true
			}
			if vd, _, _, ok := m.valueDeclOf(id); ok && vd.Modif != nil && (vd.Modif.String() == "@lazy" || vd.Modif.String() == "@fuzzy") {
				initialiser(id.String(), be.Y, vd.Deterministic != nil)
			}
		case *syntax.CommClause:
			if x.X != nil {
				deterministicUses(x.X, "an alt guard")
			}
		case *syntax.CallExpr:
			sel, ok := x.Fun.(*syntax.SelectorExpr)
			if !ok || x.Args == nil {
				return true
			}
			switch op := syntax.Name(sel.Sel); op {
			case "receive", "trigger", "check", "getcall", "getreply", "catch":
				for _, arg := range x.Args.List {
					deterministicUses(arg, "a "+op+" operation")
				}
			}
		}
		return true
	})
	return diags
}

// nonDeterministicModif is "@lazy" or "@fuzzy" when id refers to a
// variable or formal parameter of that modifier without @deterministic,
// else "".
func (m typeModel) nonDeterministicModif(id *syntax.Ident) string {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return ""
	}
	var modif, det syntax.Token
	switch d := defs[0].Node.(type) {
	case *syntax.FormalPar:
		modif, det = d.Modif, d.Deterministic
	case *syntax.ValueDecl, *syntax.Declarator:
		vd, _, _, ok := m.valueDeclOf(id)
		if !ok {
			return ""
		}
		modif, det = vd.Modif, vd.Deterministic
	default:
		return ""
	}
	if modif == nil || det != nil {
		return ""
	}
	if s := modif.String(); s == "@lazy" || s == "@fuzzy" {
		return s
	}
	return ""
}
