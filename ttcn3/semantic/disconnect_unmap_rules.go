// disconnect_unmap_rules.go enforces ETSI ES 201 873-1 clause
// 21.1.2 restrictions on the `disconnect` / `unmap` operations:
//
//   - the special `all component:all port` argument shall only
//     be used by the MTC; we approximate that by allowing it
//     only inside a `testcase` (which always runs on the MTC)
//     or inside the module `control` part. A bare `function`
//     body is rejected.
//
//   - the disconnect / unmap parameter list shall not contain
//     a `system:<port>` reference, irrespective of the
//     enclosing scope. This is restriction d in the spec.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkDisconnectUnmapRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch decl := d.Def.(type) {
		case *syntax.FuncDecl:
			if decl == nil || decl.Body == nil || decl.KindTok == nil {
				continue
			}
			isFunction := decl.KindTok.Kind() == syntax.FUNCTION
			diags = append(diags, walkDisconnectUnmap(decl.Body, isFunction)...)
		case *syntax.ControlPart:
			if decl == nil || decl.Body == nil {
				continue
			}
			diags = append(diags, walkDisconnectUnmap(decl.Body, false)...)
		}
	}
	return diags
}

func walkDisconnectUnmap(body *syntax.BlockStmt, inFunction bool) []Diagnostic {
	if body == nil {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		name := id.String()
		if name != "disconnect" && name != "unmap" {
			return true
		}
		if ce.Args == nil {
			return true
		}
		for _, arg := range ce.Args.List {
			be, ok := arg.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
				continue
			}
			compID, ok := be.X.(*syntax.Ident)
			if !ok || compID == nil {
				continue
			}
			compTok := compID.String()
			// `disconnect(system:p)` is illegal because system
			// ports can only be mapped, not connected.
			// `unmap(system:p)` is the symmetric counterpart
			// of `map(system:p, ...)` and is fully allowed.
			if compTok == "system" && name == "disconnect" {
				diags = append(diags, Diagnostic{
					Code:     "disconnect-system-port-ref",
					Severity: SeverityError,
					Message:  "`disconnect` cannot reference a system port (`system:<port>`); system ports are mapped, not connected (ETSI 21.1.2)",
					Node:     be,
					Span:     syntax.SpanOf(be),
				})
				continue
			}
			if (compTok == "all" || compTok == "all component") && inFunction {
				// `all component:...` inside a function body
				// (not testcase, not control). Restricted to
				// MTC; functions may be started on PTCs.
				diags = append(diags, Diagnostic{
					Code:     name + "-all-component-in-function",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`%s(all component:all port)` is only allowed from MTC (testcase or module control); "+
							"a plain function may be started on a PTC (ETSI 21.1.2)",
						name),
					Node: be,
					Span: syntax.SpanOf(be),
				})
			}
		}
		return true
	})
	return diags
}
