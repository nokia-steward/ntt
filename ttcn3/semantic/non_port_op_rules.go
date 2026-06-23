// non_port_op_rules.go enforces ETSI ES 201 873-1 clauses 22.2
// and 22.3 restriction "the ObjectReference shall be of a port
// type": a port communication operation must be invoked on a
// variable / instance whose declared type is one of the
// module's `type port` declarations. Invoking the op on a
// component-local `var`, `const`, or `timer` is rejected.
//
// The check is intentionally narrow:
//   - we only resolve the LHS name against the component the
//     enclosing function / testcase runs on; cross-component
//     references stay untouched;
//   - we only flag when the LHS is a known non-port member of
//     that component. Identifiers we cannot classify are left
//     alone to avoid false positives on local vars / outer-
//     scope timers.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkNonPortOpRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	compNonPorts := collectComponentNonPortMembers(mod)
	if len(compNonPorts) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		comp := runsOnComponent(fn)
		if comp == "" {
			continue
		}
		nonPorts := compNonPorts[comp]
		if len(nonPorts) == 0 {
			continue
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil {
				return true
			}
			sel, ok := ce.Fun.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			id, ok := sel.X.(*syntax.Ident)
			if !ok || id == nil {
				return true
			}
			op, ok := sel.Sel.(*syntax.Ident)
			if !ok || op == nil {
				return true
			}
			kind, isMember := nonPorts[id.String()]
			if !isMember {
				return true
			}
			if !isPortCommOp(op.String()) {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "port-op-on-non-port",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`%s.%s(...)` invokes a port operation on a %s; the receiver must be of a port type (ETSI 22.2 / 22.3)",
					id.String(), op.String(), kind),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}
	return diags
}

// collectComponentNonPortMembers returns, per component, a map of
// member-name to its declarator kind ("var", "const", "timer") for
// every non-port declaration in the component body. Inherited
// members are not modelled - extension semantics require the full
// type-graph walker, which is out of scope for this rule.
func collectComponentNonPortMembers(mod *syntax.Module) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct == nil || ct.Name == nil || ct.Body == nil {
			continue
		}
		members := map[string]string{}
		for _, stmt := range ct.Body.Stmts {
			ds, ok := stmt.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd == nil || vd.KindTok == nil {
				continue
			}
			kindStr := ""
			switch vd.KindTok.Kind() {
			case syntax.VAR:
				kindStr = "var"
			case syntax.CONST:
				kindStr = "const"
			case syntax.TIMER:
				kindStr = "timer"
			default:
				continue
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				members[dec.Name.String()] = kindStr
			}
		}
		if len(members) > 0 {
			out[ct.Name.String()] = members
		}
	}
	return out
}

// isPortCommOp reports whether `op` is one of the port
// communication operations whose receiver must be a port type.
// The synchronisation primitives (`start`, `stop`, `clear`,
// `halt`, `checkstate`) are not flagged because they share
// names with timer / component operations and would produce
// false positives.
func isPortCommOp(op string) bool {
	switch op {
	case "send", "receive", "trigger", "check",
		"call", "getcall", "reply", "getreply", "raise", "catch":
		return true
	}
	return false
}
