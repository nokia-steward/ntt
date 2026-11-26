// value_notation_rules.go checks values written for structured and string
// types (ETSI ES 201 873-1 clauses 6.1.1.1, 6.2.3, 6.2.5). The elements
// of a record of or set of have no identifiers, so its values do not
// assign them by name:
//
//	type record of integer RoI;
//	var RoI v := { field1 := 1 }; // error
//
// A union value chooses one alternative and gives it a value, neither the
// not used symbol nor omit. An element of a string is a single element,
// so a string of another length is not assigned to it.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkValueNotationRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	// value checks v, written for the type t of tree; template tells
	// whether it is a template's.
	value := func(tree *ttcn3.Tree, t syntax.Expr, v syntax.Expr, template bool) {
		cl, ok := v.(*syntax.CompositeLiteral)
		if !ok {
			return
		}
		r, ok := m.typeRoot(tree, t, 0)
		if !ok {
			return
		}
		for _, el := range cl.List {
			be, ok := el.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				continue
			}
			switch r.kind {
			case "record of", "set of":
				if id, ok := be.X.(*syntax.Ident); ok {
					add(be, "list-element-identifier", fmt.Sprintf("the elements of a %s have no identifiers: %s is not one (ETSI 6.2.3)", r.kind, id))
				}
			case "union":
				if !template && (isNotUsedSymbol(be.Y) || isOmitLiteral(be.Y)) {
					add(be.Y, "union-value-not-given", fmt.Sprintf("the alternative %s of a union value is given a value, not %s (ETSI 6.2.5)", syntax.Name(be.X), sourceText(be.Y)))
				}
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			if x.Type == nil || x.KindTok == nil {
				return true
			}
			switch x.KindTok.Kind() {
			case syntax.VAR, syntax.CONST, syntax.MODULEPAR:
			default:
				return true
			}
			for _, d := range x.Decls {
				if d != nil && d.Value != nil && len(d.ArrayDef) == 0 {
					value(tree, x.Type, d.Value, x.TemplateRestriction != nil)
				}
			}
		case *syntax.TemplateDecl:
			if x.Type != nil && x.Value != nil && x.ModifiesTok == nil {
				value(tree, x.Type, x.Value, true)
			}
		case *syntax.ExprStmt:
			be, ok := x.Expr.(*syntax.BinaryExpr)
			if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			ix, ok := be.X.(*syntax.IndexExpr)
			if !ok {
				return true
			}
			lit, ok := be.Y.(*syntax.ValueLiteral)
			if !ok {
				return true
			}
			r, ok := m.exprRoot(ix.X)
			if !ok {
				return true
			}
			switch r.kind {
			case "bitstring", "hexstring", "octetstring", "charstring", "universal charstring":
				if n, _, ok := stringLiteralLength(lit); ok && n != 1 {
					add(be.Y, "string-element-length", fmt.Sprintf("%s is an element of a %s: a string of %d elements is not assigned to it (ETSI 6.1.1.1)", sourceText(be.X), r.kind, n))
				}
			}
		}
		return true
	})
	return diags
}
