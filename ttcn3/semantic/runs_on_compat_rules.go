// runs_on_compat_rules.go checks that a function is invoked on a
// component it may run on, with an mtc and a system it may have (ETSI ES
// 201 873-1 6.3.3, 16.1.1 d, f): every definition of the component type
// of its runs on clause has an identical definition in the caller's, and
// every port definition of the types of its mtc and system clauses in the
// caller's mtc and system types. Component types are resolved by the
// tree's lookup; a call whose types do not resolve, or whose caller's mtc
// or system type is not known (16.1.0), is let be.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRunsOnCompatibility(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	db := a.db()
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		caller, ok := d.Def.(*syntax.FuncDecl)
		if !ok || caller.Body == nil || caller.KindTok == nil {
			continue
		}
		runsOn := componentDecl(tree, db, runsOnExpr(caller.RunsOn))
		mtcType, systemType := callerMtcAndSystem(tree, db, caller, runsOn)
		syntax.Inspect(caller.Body, func(n syntax.Node) bool {
			call, ok := n.(*syntax.CallExpr)
			if !ok || runsElsewhere(tree, call) {
				return true
			}
			id, ok := call.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			defs := tree.LookupWithDB(id, db)
			if len(defs) != 1 {
				return true
			}
			callee, ok := defs[0].Node.(*syntax.FuncDecl)
			if !ok || callee.KindTok == nil || callee.KindTok.Kind() != syntax.FUNCTION {
				return true
			}
			report := func(code, format string, args ...any) {
				diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: fmt.Sprintf(format, args...), Node: call, Span: syntax.SpanOf(call)})
			}
			if callee.RunsOn != nil && runsOn != nil {
				if want := componentDecl(defs[0].Tree, db, callee.RunsOn.Comp); want != nil && !componentCompatibleWith(tree, db, runsOn, want, false) {
					report("runs-on-incompatible", "%s runs on %s, which the caller's component %s does not have all definitions of (ETSI 6.3.3, 16.1.1 d)", id.String(), want.Name.String(), runsOn.Name.String())
				}
			}
			if callee.Mtc != nil && mtcType != nil {
				if want := componentDecl(defs[0].Tree, db, callee.Mtc.Comp); want != nil && !componentCompatibleWith(tree, db, mtcType, want, true) {
					report("mtc-incompatible", "%s has mtc %s, which the caller's mtc %s is not compatible with (ETSI 6.3.3, 16.1.1 f)", id.String(), want.Name.String(), mtcType.Name.String())
				}
			}
			if callee.System != nil && systemType != nil {
				if want := componentDecl(defs[0].Tree, db, callee.System.Comp); want != nil && !componentCompatibleWith(tree, db, systemType, want, true) {
					report("system-incompatible", "%s has system %s, which the caller's system %s is not compatible with (ETSI 6.3.3, 16.1.1 f)", id.String(), want.Name.String(), systemType.Name.String())
				}
			}
			return true
		})
	}
	return diags
}

func runsOnExpr(r *syntax.RunsOnSpec) syntax.Expr {
	if r == nil {
		return nil
	}
	return r.Comp
}

// callerMtcAndSystem are the mtc and system types known where caller
// runs: its mtc and system clauses, or a testcase's runs on type and,
// without a system clause, its runs on type as the system (6.3.3).
func callerMtcAndSystem(tree *ttcn3.Tree, db *ttcn3.DB, caller *syntax.FuncDecl, runsOn *syntax.ComponentTypeDecl) (mtc, system *syntax.ComponentTypeDecl) {
	if caller.Mtc != nil {
		mtc = componentDecl(tree, db, caller.Mtc.Comp)
	}
	if caller.System != nil {
		system = componentDecl(tree, db, caller.System.Comp)
	}
	if caller.KindTok.Kind() == syntax.TESTCASE {
		if mtc == nil {
			mtc = runsOn
		}
		if caller.System == nil {
			system = runsOn
		}
	}
	return mtc, system
}

// runsElsewhere reports whether call is a behaviour started on another
// component or activated as a default, which other rules check.
func runsElsewhere(tree *ttcn3.Tree, call *syntax.CallExpr) bool {
	for p := tree.ParentOf(call); p != nil; p = tree.ParentOf(p) {
		c, ok := p.(*syntax.CallExpr)
		if !ok {
			if _, ok := p.(*syntax.ParenExpr); ok {
				continue
			}
			return false
		}
		switch f := c.Fun.(type) {
		case *syntax.SelectorExpr:
			return syntax.Name(f.Sel) == "start"
		case *syntax.Ident:
			return f.String() == "activate"
		}
		return false
	}
	return false
}

// componentDecl is the component type e refers to, or nil.
func componentDecl(tree *ttcn3.Tree, db *ttcn3.DB, e syntax.Expr) *syntax.ComponentTypeDecl {
	ct, _ := componentNode(tree, db, e)
	return ct
}

// componentNode is the component type e, written in tree, refers to, and
// the tree it is declared in.
func componentNode(tree *ttcn3.Tree, db *ttcn3.DB, e syntax.Expr) (*syntax.ComponentTypeDecl, *ttcn3.Tree) {
	if e == nil {
		return nil, nil
	}
	defs := tree.LookupWithDB(e, db)
	if len(defs) != 1 {
		return nil, nil
	}
	ct, ok := defs[0].Node.(*syntax.ComponentTypeDecl)
	if !ok {
		return nil, nil
	}
	return ct, defs[0].Tree
}

// componentDef is what a definition of a component type is, as far as the
// rule compares it: its kind and, of a port, the declaration of its type.
type componentDef struct {
	kind     string
	portType syntax.Node
}

// componentCompatibleWith reports whether every definition of want, or
// with portsOnly every port definition, has a definition in have of its
// name and kind, and a port of the same port type (6.3.3 a-d). The types,
// initial values and durations of the others are not compared: their
// text differs where they do not (`M.T` and `T`, `(0)` and `0`). A
// component type whose definitions cannot all be found (an extended type
// not resolved) is compatible.
func componentCompatibleWith(tree *ttcn3.Tree, db *ttcn3.DB, have, want *syntax.ComponentTypeDecl, portsOnly bool) bool {
	if have == want {
		return true
	}
	wantDefs, ok := componentDefinitions(componentTree(tree, db, want), db, want, map[syntax.Node]bool{})
	if !ok {
		return true
	}
	haveDefs, ok := componentDefinitions(componentTree(tree, db, have), db, have, map[syntax.Node]bool{})
	if !ok {
		return true
	}
	for name, w := range wantDefs {
		if portsOnly && w.kind != "port" {
			continue
		}
		h, ok := haveDefs[name]
		if !ok || h.kind != w.kind {
			return false
		}
		if w.portType != nil && h.portType != nil && w.portType != h.portType {
			return false
		}
	}
	return true
}

// componentTree is the tree ct is declared in: tree, or the one its name
// resolves to.
func componentTree(tree *ttcn3.Tree, db *ttcn3.DB, ct *syntax.ComponentTypeDecl) *ttcn3.Tree {
	if ct.Name != nil {
		if _, t := componentNode(tree, db, ct.Name); t != nil {
			return t
		}
	}
	return tree
}

// componentDefinitions maps the name of each definition of ct, declared
// in tree, its own and those of the types it extends, to what it is, and
// reports whether all could be found.
func componentDefinitions(tree *ttcn3.Tree, db *ttcn3.DB, ct *syntax.ComponentTypeDecl, seen map[syntax.Node]bool) (map[string]componentDef, bool) {
	out := map[string]componentDef{}
	if seen[ct] {
		return out, true
	}
	seen[ct] = true
	for _, e := range ct.Extends {
		parent, ptree := componentNode(tree, db, e)
		if parent == nil {
			return nil, false
		}
		defs, ok := componentDefinitions(ptree, db, parent, seen)
		if !ok {
			return nil, false
		}
		for k, v := range defs {
			out[k] = v
		}
	}
	if ct.Body == nil {
		return out, true
	}
	for _, st := range ct.Body.Stmts {
		ds, ok := st.(*syntax.DeclStmt)
		if !ok {
			continue
		}
		switch x := ds.Decl.(type) {
		case *syntax.ValueDecl:
			def := componentDef{kind: "var"}
			if x.KindTok != nil {
				def.kind = strings.ToLower(x.KindTok.String())
			}
			if def.kind == "port" && x.Type != nil {
				if defs := tree.LookupWithDB(x.Type, db); len(defs) == 1 {
					def.portType = defs[0].Node
				}
			}
			for _, dec := range x.Decls {
				if dec != nil && dec.Name != nil {
					out[dec.Name.String()] = def
				}
			}
		case *syntax.TemplateDecl:
			if x.Name != nil {
				out[x.Name.String()] = componentDef{kind: "template"}
			}
		}
	}
	return out, true
}
