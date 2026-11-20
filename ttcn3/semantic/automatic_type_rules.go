// automatic_type_rules.go enforces ETSI ES 201 873-1 clause 6.5: a
// declaration without a type takes the type of what it is initialised
// with, which a matching symbol of no type, `?` or `*`, and `omit` do not
// have.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAutomaticTypes(mod *syntax.Module) []Diagnostic {
	enums, values := enumLabelsAndValueNames(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd.Type != nil {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Value == nil {
				continue
			}
			if why := autoTypeProblem(d.Value, enums, values, 0); why != "" {
				diags = append(diags, Diagnostic{
					Code:     "automatic-type-unknown",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s gives no type for %s declared without one (ETSI 6.5)", why, syntax.Name(d.Name)),
					Node:     d,
					Span:     syntax.SpanOf(d),
				})
			}
		}
		return true
	})
	return diags
}

// autoTypeProblem is why expression e, an initialiser of a declaration
// without a type, gives none (ETSI 6.5), or "". At the top, `?`, `*`,
// omit and `{}` have no type of their own; inside a value list, field or
// index notation, template list or set template they take the common
// type the others give, so the notation has none only when none of them
// gives one. null, a value of every reference type alike, gives no type,
// nor does an enumerated value not qualified by its type.
func autoTypeProblem(e syntax.Expr, enums, values map[string]bool, depth int) string {
	if depth > 32 {
		return ""
	}
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil {
			return ""
		}
		switch x.Tok.Kind() {
		case syntax.NULL:
			// null is a value of every reference type alike: no
			// type (ETSI NegSem_0605_automatic_type_003).
			return `"null"`
		case syntax.ANY, syntax.MUL, syntax.OMIT:
			if depth == 0 {
				return fmt.Sprintf("%q", x.Tok.String())
			}
		}
	case *syntax.Ident:
		if name := x.String(); enums[name] && !values[name] {
			return fmt.Sprintf("the enumerated value %s not qualified by its type", name)
		}
	case *syntax.CompositeLiteral:
		if len(x.List) == 0 {
			if depth == 0 {
				return "{}"
			}
			return ""
		}
		var elems []syntax.Expr
		for _, el := range x.List {
			if b, ok := el.(*syntax.BinaryExpr); ok && b.Op != nil && b.Op.Kind() == syntax.ASSIGN {
				if ix, ok := b.X.(*syntax.IndexExpr); ok && ix.X == nil {
					if why := autoTypeProblem(ix.Index, enums, values, depth+1); why != "" {
						return why
					}
				}
				el = b.Y
			}
			elems = append(elems, el)
		}
		return elementsProblem(elems, enums, values, depth)
	case *syntax.ParenExpr:
		if len(x.List) > 1 {
			return elementsProblem(x.List, enums, values, depth)
		}
	case *syntax.CallExpr:
		switch syntax.Name(x.Fun) {
		case "superset", "subset", "permutation", "complement":
			if x.Args != nil {
				return elementsProblem(x.Args.List, enums, values, depth)
			}
		}
	}
	return ""
}

// elementsProblem is autoTypeProblem of the elements of a notation: the
// problem of one, or that none gives a type.
func elementsProblem(elems []syntax.Expr, enums, values map[string]bool, depth int) string {
	typed := false
	for _, el := range elems {
		if why := autoTypeProblem(el, enums, values, depth+1); why != "" {
			return why
		}
		if !typeless(el) {
			typed = true
		}
	}
	if !typed && len(elems) > 0 {
		return "no element of the notation"
	}
	return ""
}

// typeless reports whether e has no type of its own: ?, *, omit, {}.
func typeless(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x.Tok != nil {
			switch x.Tok.Kind() {
			case syntax.ANY, syntax.MUL, syntax.OMIT:
				return true
			}
		}
	case *syntax.CompositeLiteral:
		return len(x.List) == 0
	}
	return false
}

// enumLabelsAndValueNames are the labels of the enumerated types mod
// declares, and the names it gives values, parameters and other
// definitions, which a label of the same name does not stand for.
func enumLabelsAndValueNames(mod *syntax.Module) (enums, values map[string]bool) {
	enums, values = map[string]bool{}, map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.EnumTypeDecl:
			for _, e := range x.Enums {
				enums[enumLabelName(e)] = true
			}
		case *syntax.EnumSpec:
			for _, e := range x.Enums {
				enums[enumLabelName(e)] = true
			}
		case *syntax.Declarator:
			if x.Name != nil {
				values[x.Name.String()] = true
			}
		case *syntax.FormalPar:
			if x.Name != nil {
				values[x.Name.String()] = true
			}
		case *syntax.TemplateDecl:
			if x.Name != nil {
				values[x.Name.String()] = true
			}
		case *syntax.FuncDecl:
			if x.Name != nil {
				values[x.Name.String()] = true
			}
		}
		return true
	})
	delete(enums, "")
	return enums, values
}

// enumLabelName is the label of an enumeration item, `red` or `red(1)`.
func enumLabelName(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.Ident:
		return x.String()
	case *syntax.CallExpr:
		return syntax.Name(x.Fun)
	}
	return ""
}
