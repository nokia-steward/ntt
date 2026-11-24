// regexp_rules.go checks the arguments of the predefined function regexp
// (ETSI ES 201 873-1 clause C.4.1), `regexp(inpar, expression, groupno)`:
//
//   - inpar and expression are values or templates of charstring or
//     universal charstring types;
//   - an inpar template contains the specific value matching mechanism
//     only, an expression template the specific value or pattern
//     matching mechanisms only.
//
// A charstring inpar with a universal charstring expression is left
// alone: ETSI's Sem_160102_predefined_functions_025 takes it for valid
// (resolution of CR 6424).
//
// Root types come from the root-type model; a rule fires only when they
// are known.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRegexpRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		// regexp is a keyword, its arguments a parenthesised list.
		call, ok := n.(*syntax.RegexpExpr)
		if !ok {
			return true
		}
		args, ok := call.X.(*syntax.ParenExpr)
		if !ok || len(args.List) != 3 {
			return true
		}
		inpar, expr := args.List[0], args.List[1]
		in, inOK := m.exprRoot(inpar)
		ex, exOK := m.exprRoot(expr)
		for _, arg := range []struct {
			e    syntax.Expr
			r    rootType
			ok   bool
			name string
		}{{inpar, in, inOK, "inpar"}, {expr, ex, exOK, "expression"}} {
			if arg.ok && arg.r.primitive() && !isCharKind(arg.r.kind) {
				add(arg.e, "regexp-not-a-character-string", fmt.Sprintf("regexp: %s is a %s, not a charstring or universal charstring (ETSI C.4.1)", arg.name, arg.r.kind))
				return true
			}
		}
		if mech := inlineMechanism(m.wholeArg(inpar)); mech != "" {
			add(inpar, "regexp-inpar-not-specific", fmt.Sprintf("regexp: inpar contains %s; an inpar template contains specific values only (ETSI C.4.1)", mech))
		}
		if mech := inlineMechanism(m.wholeArg(expr)); mech != "" {
			add(expr, "regexp-expression-mechanism", fmt.Sprintf("regexp: expression contains %s; an expression template contains specific values or patterns only (ETSI C.4.1)", mech))
		}
		return true
	})
	return diags
}

// wholeArg is an argument without its in-line type, `T: v`, or the body of
// the parameterless template it refers to.
func (m typeModel) wholeArg(e syntax.Expr) syntax.Expr {
	if b, ok := e.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.COLON {
		e = b.Y
	}
	if body := m.templateBody(e); body != nil {
		return body
	}
	return e
}

// inlineMechanism names the matching mechanism, other than a specific
// value or a pattern, template body e is as a whole; "" otherwise.
func inlineMechanism(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil {
			return ""
		}
		switch x.Tok.Kind() {
		case syntax.ANY:
			return "AnyValue"
		case syntax.MUL:
			return "AnyValueOrNone"
		case syntax.OMIT:
			return "omit"
		}
	case *syntax.ParenExpr:
		if len(x.List) > 1 {
			return "a value list"
		}
	case *syntax.CallExpr:
		if syntax.Name(x.Fun) == "complement" {
			return "a complemented list"
		}
	}
	return ""
}
