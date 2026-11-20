// type_rules.go enforces type compatibility rules of ETSI ES 201 873-1 on
// the root-type model (type_model.go), for the basic and string types
// only so far:
//
//   - a value assigned or given as initial value is of the target's root
//     type, charstring and universal charstring compatible (6.3.1, 19.1);
//   - the operands of a relational operator are of compatible root types
//     (7.1.3);
//   - the operands of and, or, xor and not are boolean (7.1.4);
//   - an actual parameter given to an in formal parameter of a function,
//     altstep or testcase is of its root type (5.4.2 d);
//   - the templates of the branches of a select case are of the header's
//     type (19.3.1), the header of a select union is a union (19.3.2).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTypeRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	funcs := collectFuncDeclsByName(mod)
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// primitive pair of known root types, or ok false.
	both := func(x, y syntax.Expr) (rootType, rootType, bool) {
		l, lok := m.exprRoot(x)
		r, rok := m.exprRoot(y)
		return l, r, lok && rok && l.primitive() && r.primitive()
	}
	// pair of known root types, both basic or string, or both
	// structured, for assignments.
	assignPair := func(x, y syntax.Expr) (rootType, rootType, bool) {
		l, lok := m.exprRoot(x)
		r, rok := m.exprRoot(y)
		return l, r, lok && rok && sameFamily(l, r)
	}
	// An assignment statement, not a field or parameter assignment of a
	// notation (19.1).
	checkAssign := func(st syntax.Stmt) {
		es, ok := st.(*syntax.ExprStmt)
		if !ok {
			return
		}
		x, ok := es.Expr.(*syntax.BinaryExpr)
		if !ok || x.Op == nil || x.Op.Kind() != syntax.ASSIGN {
			return
		}
		// A template is not assigned to a value variable (19.1.1).
		if m.isValueVarRef(x.X) && (m.isTemplateRef(x.Y) || isWildcardTemplate(x.Y)) {
			add(x, "template-to-value", fmt.Sprintf("%s holds a value: a template is not assigned to it (ETSI 19.1.1)", syntax.Name(x.X)))
		}
		switch target := x.X.(type) {
		case *syntax.Ident, *syntax.SelectorExpr:
			if t, v, ok := assignPair(target, x.Y); ok && !m.compatible(t, v, 0) {
				add(x, "type-incompatible-assignment", fmt.Sprintf("a %s value is assigned to the %s %s (ETSI 6.3.1, 19.1)", v.kind, t.kind, syntax.Name(target)))
			}
		}
	}
	mod.Inspect(func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ExprStmt:
			checkAssign(x)
		case *syntax.ForStmt:
			checkAssign(x.Init)
			checkAssign(x.Post)
		case *syntax.ForRangeStmt:
			// The loop variable takes the elements of the list (19.4.2).
			ds, ok := x.Init.(*syntax.DeclStmt)
			if !ok {
				return true
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd.Type == nil {
				return true
			}
			t, tok := m.typeRoot(tree, vd.Type, 0)
			el, eok := m.listElemRoot(x.Range)
			if tok && eok && sameFamily(t, el) && !m.compatible(t, el, 0) {
				add(vd, "range-loop-element-type", fmt.Sprintf("the elements of the list are %s values, not %s (ETSI 19.4.2)", el.kind, t.kind))
			}
		case *syntax.RedirectExpr:
			// A verdict redirection stores into a verdicttype (21.3.10).
			if x.Verdict != nil {
				if r, ok := m.exprRoot(x.Verdict); ok && (r.primitive() || r.structured()) && r.kind != "verdicttype" {
					add(x.Verdict, "verdict-redirect-type", fmt.Sprintf("the verdict is stored into a %s, not a verdicttype (ETSI 21.3.10)", r.kind))
				}
			}
			// A value redirection stores the message the in-line
			// template `T: ...` matches into a variable of a type T is
			// compatible with (22.2.2, 22.4).
			if len(x.Value) == 1 {
				if call, ok := x.X.(*syntax.CallExpr); ok && receivingOpName(call.Fun) != "" && call.Args != nil && len(call.Args.List) == 1 {
					if b, ok := call.Args.List[0].(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.COLON {
						t, tok := m.exprRoot(x.Value[0])
						v, vok := m.typeRoot(tree, b.X, 0)
						if tok && vok && sameFamily(t, v) && !m.compatible(t, v, 0) {
							add(x.Value[0], "value-redirect-type", fmt.Sprintf("a %s message is stored into a %s (ETSI 22.2.2)", v.kind, t.kind))
						}
					}
				}
			}
		case *syntax.ValueDecl:
			// A template variable or module parameter is initialised
			// within its restriction (15.8): template(present) excludes
			// AnyValueOrNone as a whole too.
			if restr := restrictionTag(x.TemplateRestriction); restr != "" {
				for _, d := range x.Decls {
					// A call's arguments are not the initialiser's
					// matching mechanisms: one with a call is left to
					// the run time (15.8).
					if d == nil || d.Value == nil || hasCallExpr(d.Value) {
						continue
					}
					diags = append(diags, restrictionViolationsIn(d.Value, restr, d)...)
					if v, ok := d.Value.(*syntax.ValueLiteral); ok && restr == "present" && v.Tok != nil && v.Tok.Kind() == syntax.MUL {
						add(v, "template-restriction-violation", `"*" is not allowed in a template(present) initialiser`)
					}
				}
			}
			// A value variable or constant is not initialised with a
			// template (11.1, 10).
			if x.KindTok != nil && x.TemplateRestriction == nil && (x.KindTok.Kind() == syntax.VAR || x.KindTok.Kind() == syntax.CONST) {
				for _, d := range x.Decls {
					if d != nil && d.Value != nil && (m.isTemplateRef(d.Value) || isWildcardTemplate(d.Value)) {
						add(d, "template-to-value", fmt.Sprintf("%s holds a value: a template is no initial value of it (ETSI 11.1, 10)", syntax.Name(d.Name)))
					}
				}
			}
			// A template variable's initial value is type compatible
			// too (11.2 l); its matching mechanisms have no known root
			// type.
			if x.KindTok == nil || x.Type == nil {
				return true
			}
			switch x.KindTok.Kind() {
			case syntax.VAR, syntax.CONST, syntax.MODULEPAR:
			default:
				return true
			}
			t, ok := m.typeRoot(tree, x.Type, 0)
			if !ok || !t.primitive() && !t.structured() {
				return true
			}
			for _, d := range x.Decls {
				if d == nil || d.Value == nil || len(d.ArrayDef) > 0 {
					continue
				}
				if v, ok := m.exprRoot(d.Value); ok && (v.primitive() || v.structured()) && t.primitive() == v.primitive() && !m.compatible(t, v, 0) {
					add(d, "type-incompatible-value", fmt.Sprintf("a %s value is no initial value of the %s %s (ETSI 6.3.1)", v.kind, t.kind, syntax.Name(d.Name)))
				}
			}
		case *syntax.BinaryExpr:
			if x.Op == nil {
				return true
			}
			switch x.Op.Kind() {
			case syntax.LT, syntax.GT, syntax.LE, syntax.GE, syntax.EQ, syntax.NE:
				if l, r, ok := both(x.X, x.Y); ok && !comparable(l, r) {
					add(x, "type-incompatible-comparison", fmt.Sprintf("a %s value is compared with a %s value (ETSI 7.1.3)", l.kind, r.kind))
				}
			case syntax.ADD, syntax.SUB, syntax.MUL, syntax.DIV, syntax.MOD, syntax.REM:
				// A template is no operand of an arithmetic operator (7).
				for _, side := range []syntax.Expr{x.X, x.Y} {
					if m.isTemplateRef(side) {
						add(side, "template-operand", fmt.Sprintf("template %s is no operand of %s (ETSI 7)", syntax.Name(side), x.Op.String()))
					}
				}
			case syntax.AND, syntax.OR, syntax.XOR:
				for _, side := range []syntax.Expr{x.X, x.Y} {
					if r, ok := m.exprRoot(side); ok && r.primitive() && r.kind != "boolean" {
						add(side, "type-boolean-operand", fmt.Sprintf("an operand of %s is a %s value, not boolean (ETSI 7.1.4)", x.Op.String(), r.kind))
					}
				}
			}
		case *syntax.SelectStmt:
			if x.Tag == nil {
				return true
			}
			h, ok := m.exprRoot(x.Tag)
			if !ok {
				return true
			}
			// The header of a select union is of a union type or
			// anytype (19.3.2).
			if x.Union != nil {
				if h.primitive() || h.kind == "enumerated" || h.kind == "record" || h.kind == "set" {
					add(x.Tag, "select-union-header", fmt.Sprintf("the header of select union is a %s value, not a union (ETSI 19.3.2)", h.kind))
				}
				return true
			}
			// The templates of the branches are of the header's type
			// (19.3.1).
			for _, c := range x.Body {
				if c == nil || c.Case == nil {
					continue
				}
				for _, e := range c.Case.List {
					v, ok := m.exprRoot(e)
					if !ok || !sameFamily(h, v) {
						continue
					}
					if h.primitive() && !comparable(h, v) || h.structured() && !m.compatible(h, v, 0) && !m.compatible(v, h, 0) {
						add(e, "select-case-type", fmt.Sprintf("a %s branch of a select on a %s value (ETSI 19.3.1)", v.kind, h.kind))
					}
				}
			}
		case *syntax.UnaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.NOT {
				if r, ok := m.exprRoot(x.X); ok && r.primitive() && r.kind != "boolean" {
					add(x, "type-boolean-operand", fmt.Sprintf("the operand of not is a %s value, not boolean (ETSI 7.1.4)", r.kind))
				}
			}
		case *syntax.CallExpr:
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || x.Args == nil {
				return true
			}
			switch {
			case id.String() == "match" && len(x.Args.List) == 2 && funcs["match"] == nil:
				// The first operand of match is a value, the second a
				// template of a compatible type (15.9).
				if m.isTemplateRef(rootRef(x.Args.List[0])) {
					add(x.Args.List[0], "match-value-is-template", "the first operand of match is a value, not a template (ETSI 15.9)")
				}
				l, lok := m.exprRoot(x.Args.List[0])
				r, rok := m.exprRoot(x.Args.List[1])
				if lok && rok && sameFamily(l, r) && !m.compatible(l, r, 0) && !m.compatible(r, l, 0) {
					add(x, "match-incompatible", fmt.Sprintf("match of a %s value with a %s template (ETSI 15.9)", l.kind, r.kind))
				}
			case id.String() == "valueof" && len(x.Args.List) == 1 && funcs["valueof"] == nil:
				// valueof of a template that does not resolve to a
				// specific value (15.10).
				if body := m.templateBody(x.Args.List[0]); body != nil && isWildcardTemplate(body) {
					add(x, "valueof-not-specific", fmt.Sprintf("%s does not resolve to a specific value (ETSI 15.10)", syntax.Name(x.Args.List[0])))
				}
			}
			fd := funcs[id.String()]
			if fd == nil || fd.Params == nil {
				return true
			}
			for i, arg := range x.Args.List {
				if _, named := namedArgName(arg); named || i >= len(fd.Params.List) {
					break
				}
				p := fd.Params.List[i]
				if p == nil || p.Type == nil || len(p.ArrayDef) > 0 || p.TemplateRestriction != nil {
					continue
				}
				if p.Direction != nil && p.Direction.Kind() != syntax.IN {
					continue
				}
				t, tok := m.typeRoot(tree, p.Type, 0)
				v, vok := m.exprRoot(arg)
				if tok && vok && sameFamily(t, v) && !m.compatible(t, v, 0) {
					add(arg, "type-incompatible-actual", fmt.Sprintf("a %s value is given to the %s formal parameter %s (ETSI 5.4.2 d)", v.kind, t.kind, syntax.Name(p.Name)))
				}
			}
		}
		return true
	})
	return diags
}

// hasCallExpr reports whether e calls a function or template.
func hasCallExpr(e syntax.Expr) bool {
	found := false
	syntax.Inspect(e, func(n syntax.Node) bool {
		if _, ok := n.(*syntax.CallExpr); ok {
			found = true
		}
		return !found
	})
	return found
}
