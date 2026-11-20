// concat_and_context_rules.go enforces static rules of ETSI ES 201 873-1
// on concatenations of templates and on where some statements and
// references may stand:
//
//   - 15.11: the operands of a concatenation of templates have one root
//     type; binary string operands are specific values, AnyValue without
//     a length, or AnyValue or AnyValueOrNone of a fixed length; list
//     operands are specific values, AnyValue, or AnyValueOrNone with a
//     length; character string operands are specific values, AnyValue,
//     AnyValueOrNone with a length, or patterns, all of them @nocase when
//     one is; no operand is a template list;
//   - table 15 (clause 18): no return in a control part or a testcase,
//     no mtc or system in a control part;
//   - 15.2: the not used symbol stands for a whole template only in a
//     modified template.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkConcatAndContextRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// Concatenations: each chain once, from its top.
	inner := map[*syntax.BinaryExpr]bool{}
	mod.Inspect(func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			if x.Base == nil && x.Value != nil && isNotUsedSymbol(x.Value) && x.Name != nil {
				add(x, "not-used-symbol-template", fmt.Sprintf("template %s: the not used symbol is a template only in a modified one (ETSI 15.2)", x.Name.String()))
			}
			if b, ok := x.Value.(*syntax.BinaryExpr); ok && isConcat(b) {
				if why := concatProblem(b, syntax.Name(x.Type)); why != "" {
					add(b, "concat-operand", why+" (ETSI 15.11)")
				}
				inner[b] = true
				markInner(b, inner)
			}
		case *syntax.BinaryExpr:
			if isConcat(x) && !inner[x] {
				if why := concatProblem(x, ""); why != "" {
					add(x, "concat-operand", why+" (ETSI 15.11)")
				}
				markInner(x, inner)
			}
		case *syntax.ControlPart:
			syntax.Inspect(x.Body, func(m syntax.Node) bool {
				switch y := m.(type) {
				case *syntax.FuncDecl:
					return false
				case *syntax.ReturnStmt:
					add(y, "control-part-return", "return is not allowed in a control part (ETSI 18, table 15)")
				case *syntax.Ident:
					if y.Tok != nil && (y.Tok.Kind() == syntax.MTC || y.Tok.Kind() == syntax.SYSTEM) {
						add(y, "control-part-component-ref", fmt.Sprintf("%s is not allowed in a control part (ETSI 18, table 15)", y.String()))
					}
				}
				return true
			})
		case *syntax.FuncDecl:
			if x.KindTok == nil || x.KindTok.Kind() != syntax.TESTCASE || x.Body == nil {
				return true
			}
			syntax.Inspect(x.Body, func(m syntax.Node) bool {
				if r, ok := m.(*syntax.ReturnStmt); ok {
					add(r, "testcase-return", "return is not allowed in a testcase (ETSI 18, table 15)")
				}
				return true
			})
		}
		return true
	})
	return diags
}

func isConcat(b *syntax.BinaryExpr) bool {
	return b != nil && b.Op != nil && b.Op.Kind() == syntax.CONCAT
}

func isNotUsedSymbol(e syntax.Expr) bool {
	v, ok := e.(*syntax.ValueLiteral)
	return ok && v.Tok != nil && v.Tok.Kind() == syntax.SUB
}

// markInner marks the concatenations under the chain b, which b's check
// covers.
func markInner(b *syntax.BinaryExpr, inner map[*syntax.BinaryExpr]bool) {
	for _, side := range []syntax.Expr{b.X, b.Y} {
		if c, ok := unparen(side).(*syntax.BinaryExpr); ok && isConcat(c) {
			inner[c] = true
			markInner(c, inner)
		}
	}
}

func unparen(e syntax.Expr) syntax.Expr {
	for {
		p, ok := e.(*syntax.ParenExpr)
		if !ok || len(p.List) != 1 {
			return e
		}
		e = p.List[0]
	}
}

// concatOperands are the operands of the chain of concatenations b.
func concatOperands(e syntax.Expr) []syntax.Expr {
	if b, ok := unparen(e).(*syntax.BinaryExpr); ok && isConcat(b) {
		return append(concatOperands(b.X), concatOperands(b.Y)...)
	}
	return []syntax.Expr{unparen(e)}
}

// concatProblem is why the concatenation b, of a template of the type
// named typ when known, breaks 15.11, or "".
func concatProblem(b *syntax.BinaryExpr, typ string) string {
	ops := concatOperands(b)
	kinds := map[string]bool{} // "char", "B", "H", "O", "list"
	type wild struct {
		star, sized, fixed bool
	}
	var wilds []wild
	patterns, nocase := 0, 0
	for _, op := range ops {
		switch x := op.(type) {
		case *syntax.ValueLiteral:
			if x.Tok == nil {
				continue
			}
			switch x.Tok.Kind() {
			case syntax.STRING:
				kinds["char"] = true
			case syntax.BSTRING:
				s := x.Tok.String()
				kinds[strings.ToUpper(s[len(s)-1:])] = true
			case syntax.ANY:
				wilds = append(wilds, wild{})
			case syntax.MUL:
				wilds = append(wilds, wild{star: true})
			}
		case *syntax.LengthExpr:
			v, ok := x.X.(*syntax.ValueLiteral)
			if !ok || v.Tok == nil || v.Tok.Kind() != syntax.ANY && v.Tok.Kind() != syntax.MUL {
				continue
			}
			wilds = append(wilds, wild{star: v.Tok.Kind() == syntax.MUL, sized: true, fixed: fixedLength(x.Size)})
		case *syntax.PatternExpr:
			kinds["char"] = true
			patterns++
			if x.NoCase != nil {
				nocase++
			}
		case *syntax.CompositeLiteral:
			kinds["list"] = true
		case *syntax.ParenExpr:
			if len(x.List) > 1 {
				return "a template list is no operand of a concatenation"
			}
		}
	}
	if len(kinds) > 1 {
		return "the operands of the concatenation are of different root types"
	}
	switch {
	case kinds["B"] || kinds["H"] || kinds["O"]:
		if isCharType(typ) {
			return "a binary string concatenation is no " + typ + " template"
		}
		for _, w := range wilds {
			if w.sized && !w.fixed {
				return "a wildcard concatenated with binary strings has a fixed length or none"
			}
			if w.star && !w.sized {
				return "AnyValueOrNone concatenated with binary strings has a fixed length"
			}
		}
	case kinds["list"]:
		for _, w := range wilds {
			if w.star && !w.sized {
				return "AnyValueOrNone concatenated with lists has a length"
			}
		}
	case kinds["char"]:
		for _, w := range wilds {
			if w.star && !w.sized {
				return "AnyValueOrNone concatenated with character strings has a length"
			}
		}
		if nocase > 0 && (nocase != patterns || patterns != len(ops)) {
			return "with a pattern @nocase, every operand is a pattern @nocase"
		}
	}
	return ""
}

func isCharType(typ string) bool {
	return typ == "charstring" || typ == "universal charstring"
}

// fixedLength reports whether the length attribute size is one length.
func fixedLength(size *syntax.ParenExpr) bool {
	if size == nil || len(size.List) != 1 {
		return false
	}
	b, ok := size.List[0].(*syntax.BinaryExpr)
	if !ok || b.Op == nil || b.Op.Kind() != syntax.RANGE {
		return true
	}
	l, lok := b.X.(*syntax.ValueLiteral)
	r, rok := b.Y.(*syntax.ValueLiteral)
	return lok && rok && l.Tok != nil && r.Tok != nil && l.Tok.String() == r.Tok.String()
}
