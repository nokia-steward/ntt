// statement_operand_rules.go checks operands of statements and
// operations (ETSI ES 201 873-1 clauses 19.1.1, 19.11, 20.5.3, 24.2).
// omit is assigned to optional fields only, never to a mandatory field
// of a value:
//
//	type record R { integer a, integer b optional }
//	v_rec.a := omit; // error
//
// A running, alive, done or killed operation logged on a timer or
// component array names one of its elements. The suspend expression of
// deactivate is a boolean value, not a template. A function without a
// return clause yields nothing to log, nor to give setverdict as a
// reason.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkStatementOperandRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// logged checks an argument of log, or a reason of setverdict.
	logged := func(fn string, arg syntax.Expr) {
		if ce, ok := arg.(*syntax.CallExpr); ok {
			if f := m.funcDecl(ce.Fun); f != nil && f.Return == nil && f.KindTok != nil && f.KindTok.Kind() == syntax.FUNCTION {
				add(arg, "log-no-return-function", fmt.Sprintf("%s has no return clause: its call yields nothing for %s (ETSI 19.11, 24.2)", syntax.Name(ce.Fun), fn))
			}
			return
		}
		sel, ok := arg.(*syntax.SelectorExpr)
		if !ok {
			return
		}
		switch syntax.Name(sel.Sel) {
		case "running", "alive", "done", "killed":
		default:
			return
		}
		if id, ok := sel.X.(*syntax.Ident); ok && m.isArrayRef(id) {
			add(arg, "log-array-element-missing", fmt.Sprintf("%s is an array: %s names one of its elements (ETSI 19.11)", id, syntax.Name(sel.Sel)))
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ExprStmt:
			be, ok := x.Expr.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN || !isOmitLiteral(be.Y) {
				return true
			}
			sel, ok := be.X.(*syntax.SelectorExpr)
			if !ok || !m.isValueRef(rootRef(sel.X)) {
				return true
			}
			r, ok := m.exprRoot(sel.X)
			if !ok || r.decl == nil || r.kind != "record" && r.kind != "set" {
				return true
			}
			name := syntax.Name(sel.Sel)
			for _, f := range r.decl.Fields {
				if f != nil && syntax.Name(f.Name) == name && f.Optional == nil {
					add(be.Y, "omit-mandatory-field", fmt.Sprintf("%s is a mandatory field of %s: omit is assigned to optional fields only (ETSI 19.1.1 d)", name, r.name))
				}
			}
		case *syntax.CallExpr:
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || x.Args == nil || len(tree.LookupWithDB(id, m.db)) > 0 {
				return true
			}
			switch id.String() {
			case "log":
				for _, arg := range x.Args.List {
					logged("log", arg)
				}
			case "setverdict":
				for _, arg := range x.Args.List[min(1, len(x.Args.List)):] {
					logged("setverdict", arg)
				}
			case "deactivate":
				if len(x.Args.List) == 2 && m.isTemplateRef(x.Args.List[1]) {
					add(x.Args.List[1], "deactivate-suspend-template", fmt.Sprintf("the suspend expression %s of deactivate is a boolean value, not a template (ETSI 20.5.3 b)", sourceText(x.Args.List[1])))
				}
			}
		}
		return true
	})
	return diags
}

// funcDecl is the function, altstep or testcase e names, or nil.
func (m typeModel) funcDecl(e syntax.Expr) *syntax.FuncDecl {
	id, ok := e.(*syntax.Ident)
	if !ok {
		return nil
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return nil
	}
	f, _ := defs[0].Node.(*syntax.FuncDecl)
	return f
}

// isValueRef reports whether e is a variable or constant that is no
// template.
func (m typeModel) isValueRef(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	if !ok {
		return false
	}
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return false
	}
	vd, ok := defs[0].Node.(*syntax.ValueDecl)
	if d, isDecl := defs[0].Node.(*syntax.Declarator); isDecl {
		vd, ok = defs[0].ParentOf(d).(*syntax.ValueDecl)
	}
	if !ok || vd.KindTok == nil || vd.TemplateRestriction != nil {
		return false
	}
	k := vd.KindTok.Kind()
	return k == syntax.VAR || k == syntax.CONST
}

// isArrayRef reports whether id is a variable, constant or timer
// declared as an array.
func (m typeModel) isArrayRef(id *syntax.Ident) bool {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return false
	}
	switch d := defs[0].Node.(type) {
	case *syntax.Declarator:
		return len(d.ArrayDef) > 0
	case *syntax.ValueDecl:
		for _, x := range d.Decls {
			if x != nil && syntax.Name(x.Name) == id.String() {
				return len(x.ArrayDef) > 0
			}
		}
	}
	return false
}
