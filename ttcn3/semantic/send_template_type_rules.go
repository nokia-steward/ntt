// send_template_type_rules.go enforces ETSI ES 201 873-1 clauses
// 22.2.1 h (send), 22.2.2 o (receive / check), 22.2.3 l (trigger):
// the TemplateInstance argument of a message-based port comm op
// shall be of a data type. Component, port, timer or default
// references are forbidden.
//
// We only fire when the template argument is a bare identifier
// whose declared type we can statically resolve - call
// expressions, composite literals and any kind of computed value
// are left to the runtime.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// msgOpClauses maps each port-side operation name covered by this
// rule to the spec clause we cite in the diagnostic.
var msgOpClauses = map[string]string{
	"send":    "ETSI 22.2.1 h",
	"receive": "ETSI 22.2.2 o",
	"check":   "ETSI 22.2.2 o",
	"trigger": "ETSI 22.2.3 l",
}

func (a *Analyzer) checkSendTemplateTypeRules(mod *syntax.Module) []Diagnostic {
	components := collectComponentTypeNames(mod)
	ports := collectPortTypeNames(mod)
	if len(components) == 0 && len(ports) == 0 {
		return nil
	}
	varTypes := collectModuleVarTypes(mod)
	if len(varTypes) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || sel == nil || sel.Sel == nil {
			return true
		}
		id, ok := sel.Sel.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			return true
		}
		op := id.String()
		clause, ok := msgOpClauses[op]
		if !ok {
			return true
		}
		arg := ce.Args.List[0]
		name := identName(arg)
		if name == "" {
			return true
		}
		ty := varTypes[name]
		if ty == "" {
			return true
		}
		var kind string
		switch {
		case components[ty]:
			kind = "component"
		case ports[ty]:
			kind = "port"
		case ty == "timer" || ty == "default":
			kind = ty
		default:
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     op + "-non-data-arg",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s template argument %q has %s type %q; expected a data type (%s)",
				op, name, kind, ty, clause),
			Node: arg,
			Span: syntax.SpanOf(arg),
		})
		return true
	})
	return diags
}
