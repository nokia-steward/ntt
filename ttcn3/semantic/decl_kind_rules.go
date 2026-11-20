// decl_kind_rules.go checks the types declarations of some kinds may have,
// by what the type is, or holds at any level of nesting: a component, a
// port, a timer or a default (ETSI ES 201 873-1 5.4.1.1 m, 8.2.1 b,
// 16.1.5 b, c, d). The types are resolved through the tree's own lookup;
// one that does not resolve is taken to hold none.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// heldKind is what a type is or holds that some declarations may not.
type heldKind int

const (
	holdsComponent heldKind = 1 << iota
	holdsPort
	holdsTimer
	holdsDefault
)

func (k heldKind) String() string {
	var s []string
	for _, n := range []struct {
		k    heldKind
		name string
	}{{holdsComponent, "component"}, {holdsPort, "port"}, {holdsTimer, "timer"}, {holdsDefault, "default"}} {
		if k&n.k != 0 {
			s = append(s, n.name)
		}
	}
	return strings.Join(s, ", ")
}

func (a *Analyzer) checkDeclKinds(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	db := a.db()
	var diags []Diagnostic
	report := func(n syntax.Node, code, format string, args ...any) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: fmt.Sprintf(format, args...), Node: n, Span: syntax.SpanOf(n)})
	}
	modulepar := func(vd *syntax.ValueDecl) {
		if k := typeHolds(tree, db, vd.Type, map[syntax.Node]bool{}); k != 0 {
			report(vd, "modulepar-kind", "a module parameter shall not be of, or hold, a %s type (ETSI 8.2.1 b)", k)
		}
	}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.ValueDecl:
			if x.KindTok != nil && x.KindTok.Kind() == syntax.MODULEPAR {
				modulepar(x)
			}
		case *syntax.ModuleParameterGroup:
			for _, vd := range x.Decls {
				modulepar(vd)
			}
		case *syntax.TemplateDecl:
			// Only functions, altsteps and testcases take parameters of
			// a component type (5.4.1.1 m).
			if x.Params != nil {
				for _, p := range x.Params.List {
					if p != nil && typeHolds(tree, db, p.Type, map[syntax.Node]bool{})&holdsComponent != 0 {
						report(p, "template-param-kind", "a template's parameter shall not be of, or hold, a component type (ETSI 5.4.1.1 m)")
					}
				}
			}
		case *syntax.FuncDecl:
			if isControlFunction(x) {
				diags = append(diags, checkControlFunction(tree, db, x)...)
			}
		}
	}
	return diags
}

// isControlFunction reports whether fn is an explicit control function:
// a function named control or with the @control modifier (16.1.5).
func isControlFunction(fn *syntax.FuncDecl) bool {
	if fn.KindTok == nil || fn.KindTok.Kind() != syntax.FUNCTION {
		return false
	}
	return syntax.Name(fn.Name) == "control" || fn.Modif != nil && fn.Modif.String() == "@control"
}

func checkControlFunction(tree *ttcn3.Tree, db *ttcn3.DB, fn *syntax.FuncDecl) []Diagnostic {
	var diags []Diagnostic
	report := func(n syntax.Node, format string, args ...any) {
		diags = append(diags, Diagnostic{Code: "control-function-kind", Severity: SeverityError, Message: fmt.Sprintf(format, args...), Node: n, Span: syntax.SpanOf(n)})
	}
	if fn.Params != nil {
		for _, p := range fn.Params.List {
			if p == nil {
				continue
			}
			if k := typeHolds(tree, db, p.Type, map[syntax.Node]bool{}) & (holdsPort | holdsDefault); k != 0 {
				report(p, "a control function's parameter shall not be of, or hold, a %s type (ETSI 16.1.5 c, d)", k)
			}
		}
	}
	if fn.Return != nil {
		if k := typeHolds(tree, db, fn.Return.Type, map[syntax.Node]bool{}) & (holdsPort | holdsDefault); k != 0 {
			report(fn.Return, "a control function's return type shall not be of, or hold, a %s type (ETSI 16.1.5 c, d)", k)
		}
	}
	if fn.RunsOn != nil {
		for _, def := range tree.LookupWithDB(fn.RunsOn.Comp, db) {
			if ct, ok := def.Node.(*syntax.ComponentTypeDecl); ok && declaresPort(ct) {
				report(fn.RunsOn, "the component type a control function runs on shall declare no port (ETSI 16.1.5 b)")
			}
		}
	}
	return diags
}

// declaresPort reports whether the component type ct declares a port in
// its own body.
func declaresPort(ct *syntax.ComponentTypeDecl) bool {
	if ct.Body == nil {
		return false
	}
	for _, st := range ct.Body.Stmts {
		if ds, ok := st.(*syntax.DeclStmt); ok {
			if vd, ok := ds.Decl.(*syntax.ValueDecl); ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.PORT {
				return true
			}
		}
	}
	return false
}

// typeHolds is what the type t is, or holds in a field or an element at
// any level of nesting, of the kinds of heldKind. seen guards against a
// recursive type.
func typeHolds(tree *ttcn3.Tree, db *ttcn3.DB, t syntax.Node, seen map[syntax.Node]bool) heldKind {
	if t == nil {
		return 0
	}
	if r, ok := t.(*syntax.RefSpec); ok {
		t = r.X
	}
	switch x := t.(type) {
	case *syntax.Ident:
		switch x.String() {
		case "timer":
			return holdsTimer
		case "default":
			return holdsDefault
		}
	case *syntax.ListSpec:
		return typeHolds(tree, db, x.ElemType, seen)
	case *syntax.StructSpec:
		return fieldsHold(tree, db, x.Fields, seen)
	}
	e, ok := t.(syntax.Expr)
	if !ok {
		return 0
	}
	defs := tree.LookupWithDB(e, db)
	if len(defs) != 1 {
		return 0
	}
	def := defs[0]
	if seen[def.Node] {
		return 0
	}
	seen[def.Node] = true
	switch d := def.Node.(type) {
	case *syntax.ComponentTypeDecl:
		return holdsComponent
	case *syntax.PortTypeDecl:
		return holdsPort
	case *syntax.StructTypeDecl:
		return fieldsHold(def.Tree, db, d.Fields, seen)
	case *syntax.SubTypeDecl:
		if d.Field != nil {
			return typeHolds(def.Tree, db, d.Field.Type, seen)
		}
	case *syntax.Field:
		// A subtype's or a list type's name resolves to its field.
		return typeHolds(def.Tree, db, d.Type, seen)
	}
	return 0
}

func fieldsHold(tree *ttcn3.Tree, db *ttcn3.DB, fields []*syntax.Field, seen map[syntax.Node]bool) heldKind {
	var k heldKind
	for _, f := range fields {
		if f != nil {
			k |= typeHolds(tree, db, f.Type, seen)
		}
	}
	return k
}
