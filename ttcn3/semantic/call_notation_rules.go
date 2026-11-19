// call_notation_rules.go enforces three static rules of ETSI ES 201
// 873-1 on notations:
//
//   - a template without parameters is declared without parentheses
//     (5.4.2: "In all other cases the empty parentheses shall be
//     omitted");
//   - an actual parameter of a function, altstep or testcase is no
//     invocation of a function without a return value (5.4.2);
//   - a comparison with a compound expression on both sides compares
//     value list notations (7.1.3).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkCallNotationRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	funcs := collectFuncDeclsByName(mod)
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	mod.Inspect(func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			if x.Params != nil && len(x.Params.List) == 0 && x.Name != nil {
				add(x, "template-empty-parentheses", fmt.Sprintf("template %s has no parameters: it is declared without parentheses (ETSI 5.4.2)", x.Name.String()))
			}
		case *syntax.CallExpr:
			// The actual parameters of a function, altstep or testcase
			// of the module: those of start, call, activate and the
			// like are behaviours, not values.
			callee, ok := x.Fun.(*syntax.Ident)
			if !ok || x.Args == nil {
				return true
			}
			if _, ok := funcs[callee.String()]; !ok {
				return true
			}
			for _, arg := range x.Args.List {
				if b, ok := arg.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.ASSIGN {
					arg = b.Y
				}
				c, ok := arg.(*syntax.CallExpr)
				if !ok {
					continue
				}
				id, ok := c.Fun.(*syntax.Ident)
				if !ok {
					continue
				}
				if fd, ok := funcs[id.String()]; ok && fd.KindTok != nil && fd.KindTok.Kind() == syntax.FUNCTION && fd.Return == nil && fd.External == nil {
					add(c, "void-function-actual-parameter", fmt.Sprintf("%s returns no value: it is no actual parameter (ETSI 5.4.2)", id.String()))
				}
			}
		case *syntax.BinaryExpr:
			if x.Op == nil {
				return true
			}
			switch x.Op.Kind() {
			case syntax.EQ, syntax.NE, syntax.LT, syntax.GT, syntax.LE, syntax.GE:
			default:
				return true
			}
			l, lok := x.X.(*syntax.CompositeLiteral)
			r, rok := x.Y.(*syntax.CompositeLiteral)
			if lok && rok && (!valueListNotation(l) || !valueListNotation(r)) {
				add(x, "compound-comparison-notation", "a compound expression on both sides of a comparison is a value list notation on both (ETSI 7.1.3)")
			}
		}
		return true
	})
	return diags
}

// valueListNotation reports whether lit lists its values, with no field
// or index assignment.
func valueListNotation(lit *syntax.CompositeLiteral) bool {
	for _, e := range lit.List {
		if b, ok := e.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.ASSIGN {
			return false
		}
	}
	return true
}
