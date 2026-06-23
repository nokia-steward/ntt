// port_var_init_rules.go enforces ETSI ES 201 873-1 clause 6.3.6
// "Compatibility of port types":
//
//   - A port-typed variable / constant / formal parameter may only
//     be initialised / assigned with a value whose declared port
//     type is identical (or a typedef synonym) of its own port
//     type. In particular:
//
//       - `var P v := self;`  (a component reference)  is invalid.
//       - `var P2 v := p;`    where `p` is `port P p` and P!=P2 is
//         invalid.
//
// We only flag explicit `self`-init and identifier-init shapes
// whose source variable has a known module-local port-type
// declaration. Anything else (function-call results, complex
// expressions, parameterised assignments) is left alone.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPortVarInitRules(mod *syntax.Module) []Diagnostic {
	portTypes := collectPortTypes(mod)
	if len(portTypes) == 0 {
		return nil
	}
	varPortTypes := collectAllPortVarTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		destPort := identName(vd.Type)
		if destPort == "" {
			return true
		}
		if _, isPort := portTypes[destPort]; !isPort {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Value == nil {
				continue
			}
			if id, ok := d.Value.(*syntax.Ident); ok && id != nil {
				switch id.String() {
				case "self", "mtc", "system":
					diags = append(diags, Diagnostic{
						Code:     "port-var-init-non-port",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"port-typed variable %q cannot be initialised from %q (a component reference, ETSI 6.3.6)",
							identName(d.Name), id.String()),
						Node: vd,
						Span: syntax.SpanOf(vd),
					})
					continue
				}
				srcPort, ok := varPortTypes[id.String()]
				if !ok || srcPort == "" {
					continue
				}
				if srcPort == destPort {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "port-var-init-type-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"port-typed variable %q (port type %q) initialised from port %q of type %q (ETSI 6.3.6)",
						identName(d.Name), destPort, id.String(), srcPort),
					Node: vd,
					Span: syntax.SpanOf(vd),
				})
			}
		}
		return true
	})
	return diags
}

// collectAllPortVarTypes returns a name -> port-type map for every
// `port P name` declared inside a component body. Used to identify
// the source side of a port-init assignment.
func collectAllPortVarTypes(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	for _, ports := range collectComponentPorts(mod) {
		for name, pt := range ports {
			out[name] = pt
		}
	}
	return out
}
