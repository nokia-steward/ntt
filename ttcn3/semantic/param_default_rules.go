// param_default_rules.go checks the default values of formal parameters
// of component, port, timer and default types (ETSI ES 201 873-1 clauses
// 5.4.1.1 f and g, 5.4.1.2 e):
//
//   - the default of a component type parameter is one of the special
//     values null, mtc, self or system, a template one is built from
//     them;
//   - the default of a port, timer or default type parameter is null.
//
// A default the model cannot tell apart from those, such as a reference
// it does not resolve, is not reported.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkParamDefaultRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		p, ok := n.(*syntax.FormalPar)
		if !ok || p.Value == nil || p.Name == nil || len(p.ArrayDef) > 0 {
			return true
		}
		kind := m.paramObjectKind(p.Type)
		if kind == "" {
			return true
		}
		var bad syntax.Expr
		if kind == "component" {
			bad = m.nonSpecialComponentValue(p.Value)
		} else if !isNullLiteral(p.Value) {
			bad = p.Value
		}
		if bad == nil {
			return true
		}
		msg := fmt.Sprintf("default of %s parameter %s: %s is not null (ETSI 5.4.1.1 g)", kind, p.Name, sourceText(bad))
		if kind == "component" {
			msg = fmt.Sprintf("default of component parameter %s: %s is not null, mtc, self or system (ETSI 5.4.1.1 f, 5.4.1.2 e)", p.Name, sourceText(bad))
		}
		diags = append(diags, Diagnostic{
			Code:     "param-default-not-special-value",
			Severity: SeverityError,
			Message:  msg,
			Node:     bad,
			Span:     syntax.SpanOf(bad),
		})
		return true
	})
	return diags
}

// paramObjectKind is "component", "port", "timer" or "default" for a
// formal parameter of type t of that kind, "" otherwise.
func (m typeModel) paramObjectKind(t syntax.Expr) string {
	if isTimerType(t) {
		return "timer"
	}
	if syntax.Name(t) == "default" {
		return "default"
	}
	if _, ok := t.(*syntax.Ident); !ok {
		return ""
	}
	defs := m.tree.LookupWithDB(t, m.db)
	if len(defs) != 1 {
		return ""
	}
	switch defs[0].Node.(type) {
	case *syntax.ComponentTypeDecl:
		return "component"
	case *syntax.PortTypeDecl:
		return "port"
	}
	return ""
}

// nonSpecialComponentValue is the part of the component default e that
// is a call or a reference to a definition, through template lists and
// complements; nil when there is none.
func (m typeModel) nonSpecialComponentValue(e syntax.Expr) syntax.Expr {
	switch x := e.(type) {
	case *syntax.ParenExpr:
		for _, y := range x.List {
			if bad := m.nonSpecialComponentValue(y); bad != nil {
				return bad
			}
		}
	case *syntax.CallExpr:
		if syntax.Name(x.Fun) == "complement" && x.Args != nil {
			for _, y := range x.Args.List {
				if bad := m.nonSpecialComponentValue(y); bad != nil {
					return bad
				}
			}
			return nil
		}
		return x
	case *syntax.SelectorExpr:
		// C.create, v.f
		return x
	case *syntax.Ident:
		if len(m.tree.LookupWithDB(x, m.db)) > 0 {
			return x
		}
	}
	return nil
}
