// variadic_param_rules.go enforces ETSI ES 201 873-1 clause 5.4.3: a
// variadic formal parameter is the last of its list, the only one, and an
// in parameter; an actual parameter in assignment notation is not expanded
// with an ellipsis.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkVariadicParams(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	report := func(n syntax.Node, msg string) {
		diags = append(diags, Diagnostic{
			Code:     "variadic-parameter",
			Severity: SeverityError,
			Message:  msg + " (ETSI 5.4.3)",
			Node:     n,
			Span:     syntax.SpanOf(n),
		})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.FormalPars:
			seen := false
			for i, p := range x.List {
				if p == nil || p.Variadic == nil {
					continue
				}
				switch {
				case seen:
					report(p, "only one formal parameter of a list can be variadic")
				case i != len(x.List)-1:
					report(p, "a variadic formal parameter shall be the last of its list")
				}
				if p.Direction != nil && p.Direction.Kind() != syntax.IN {
					report(p, "a variadic formal parameter shall be an in parameter")
				}
				seen = true
			}
		case *syntax.CallExpr:
			if x.Args == nil {
				return true
			}
			for _, arg := range x.Args.List {
				b, ok := arg.(*syntax.BinaryExpr)
				if !ok || b.Op == nil || b.Op.Kind() != syntax.ASSIGN {
					continue
				}
				if pe, ok := b.Y.(*syntax.PostExpr); ok && pe.Op != nil && pe.Op.Kind() == syntax.ELIPSIS {
					report(arg, "an actual parameter in assignment notation cannot be expanded with '...'")
				}
			}
		}
		return true
	})
	return diags
}
