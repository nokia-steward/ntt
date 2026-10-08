// port_op_receiver.go enforces ETSI ES 201 873-1 clauses 22.2 and
// 22.3 by rejecting port operations applied to non-port receivers.
//
// The check is the mirror image of component_op_receiver.go: instead
// of looking for component operations on non-component receivers we
// look for port operations on receivers we can prove are NOT ports.
// We deliberately exclude the ambiguous set (`start`, `stop`, `kill`,
// `running`, `alive`) which is shared with the component-op check;
// only the operations defined exclusively on ports are flagged here.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// portOpsBuiltin lists the receiver-bound operations that ETSI
// 22.2 / 22.3 defines exclusively on ports. They never apply to a
// component or timer, so a non-port receiver is unambiguously
// wrong.
var portOpsBuiltin = map[string]bool{
	"send":     true,
	"receive":  true,
	"trigger":  true,
	"reply":    true,
	"getcall":  true,
	"getreply": true,
	"raise":    true,
	// `call`, `check`, `catch`, `clear`, `halt`, `checkstate` are
	// intentionally NOT flagged here. `call` collides with the
	// 21.3.10 test-component-call operation; the others
	// regress at least one positive Sem test in the suite when
	// the receiver is a component reference rather than a bare
	// port.
}

func (a *Analyzer) checkPortOpReceivers(mod *syntax.Module) []Diagnostic {
	declKinds := collectDeclaredTypeKinds(mod)
	if len(declKinds) == 0 {
		return nil
	}
	compMembers := collectComponentMemberTypes(mod)
	funcReturns := collectFuncReturnTypes(mod)

	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectVarDeclaredTypeNames(fn.Body)
		// Inherit the component's member-var types so port
		// operations on `p` (an anytype member of the running-on
		// component) get flagged like in
		// NegSem_220203_TriggerOperation_025.
		runsOn := ""
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			runsOn = syntax.Name(fn.RunsOn.Comp)
		}
		if runsOn != "" {
			for name, typ := range compMembers[runsOn] {
				if _, shadowed := varTypes[name]; !shadowed {
					varTypes[name] = typ
				}
			}
		}
		paramTypes := collectFormalParamTypeNames(fn.Params)
		diags = append(diags, checkPortOpReceiverInBody(
			fn.Body, varTypes, paramTypes, declKinds, funcReturns,
		)...)
	}
	return diags
}

// collectComponentMemberTypes returns the per-component map of
// `member-name -> declared-type-name` for every var declared in a
// component body. Ports are skipped because the port-instance
// resolver already handles them.
func collectComponentMemberTypes(mod *syntax.Module) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil || ct.Body == nil {
			continue
		}
		members := map[string]string{}
		for _, stmt := range ct.Body.Stmts {
			ds, ok := stmt.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd.KindTok == nil {
				continue
			}
			id, ok := vd.Type.(*syntax.Ident)
			if !ok {
				continue
			}
			typeName := id.String()
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				members[dec.Name.String()] = typeName
			}
		}
		out[ct.Name.String()] = members
	}
	return out
}

func checkPortOpReceiverInBody(
	body *syntax.BlockStmt,
	varTypes, paramTypes map[string]string,
	declKinds map[string]typeKind,
	funcReturns map[string]string,
) []Diagnostic {
	// Pre-collect every selector that's used as the function of
	// a CallExpr so the second pass can skip it and avoid
	// duplicate diagnostics (the inspector visits a selector
	// once as the call's Fun and again on its own).
	wrappedSel := map[syntax.Node]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if ce, ok := n.(*syntax.CallExpr); ok {
			if s, ok := ce.Fun.(*syntax.SelectorExpr); ok {
				wrappedSel[s] = true
			}
		}
		return true
	})

	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		var sel *syntax.SelectorExpr
		switch v := n.(type) {
		case *syntax.SelectorExpr:
			if wrappedSel[v] {
				return true
			}
			sel = v
		case *syntax.CallExpr:
			s, ok := v.Fun.(*syntax.SelectorExpr)
			if !ok {
				return true
			}
			sel = s
		default:
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		op := opIdent.String()
		if !portOpsBuiltin[op] {
			return true
		}
		switch recv := sel.X.(type) {
		case *syntax.Ident:
			name := recv.String()
			switch name {
			case "any", "all", "mtc", "self", "system":
				return true
			}
			if typeName, ok := varTypes[name]; ok {
				if checkNonPortRecv(declKinds, typeName) {
					diags = append(diags, nonPortRecvDiag(op, name, typeName, "variable", sel))
				}
				return true
			}
			if typeName, ok := paramTypes[name]; ok {
				if checkNonPortRecv(declKinds, typeName) {
					diags = append(diags, nonPortRecvDiag(op, name, typeName, "parameter", sel))
				}
				return true
			}
			if kind, ok := declKinds[name]; ok && kind != tkPort {
				diags = append(diags, nonPortRecvDiag(op, name, name, "type name", sel))
			}
		case *syntax.CallExpr:
			fnIdent, ok := recv.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			fname := fnIdent.String()
			ret, known := funcReturns[fname]
			if !known || ret == "" {
				return true
			}
			if checkNonPortRecv(declKinds, ret) {
				diags = append(diags, nonPortRecvDiag(
					op, fname+"()", ret, "function return", sel))
			}
		}
		return true
	})
	return diags
}

// builtinNonPortTypeNames enumerates a small set of built-in type
// names that we *know* are not ports, even when the module never
// declares them. `anytype` is the canonical example: it's an alias
// for "any data value" but never a port instance.
var builtinNonPortTypeNames = map[string]bool{
	"anytype":            true,
	"integer":            true,
	"boolean":            true,
	"float":              true,
	"charstring":         true,
	"bitstring":          true,
	"hexstring":          true,
	"octetstring":        true,
	"universal":          true,
	"verdicttype":        true,
	"default":            true,
	"timer":              true,
	"address":            true,
	"object":             true,
	"objectIdentifier":   true,
}

func checkNonPortRecv(declKinds map[string]typeKind, typeName string) bool {
	if kind, known := declKinds[typeName]; known {
		return kind != tkPort
	}
	return builtinNonPortTypeNames[typeName]
}

func nonPortRecvDiag(op, recvName, typeName, role string, node syntax.Node) Diagnostic {
	return Diagnostic{
		Code:     "port-op-non-port-receiver",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s on %s %q: receiver type %q is not a port (ETSI 22.2/22.3)",
			op, role, recvName, typeName),
		Node: node,
		Span: syntax.SpanOf(node),
	}
}
