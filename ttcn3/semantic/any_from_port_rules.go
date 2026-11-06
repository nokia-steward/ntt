// any_from_port_rules.go enforces ETSI ES 201 873-1 clause 22.2.2 /
// 22.3.x restriction g on the `any from` / `all from` forms of
// port communication operations:
//
//	[] any from <X>.<port-op>(...) { ... }
//	[] all from <X>.<port-op>(...) { ... }
//
// where `<port-op>` is one of receive / trigger / check / getcall
// / getreply / catch. The receiver `<X>` must be a port-array
// variable identifier - i.e. an instance declared with one or
// more array dimensions in the enclosing component's port list.
// Bare port instances (single ports) and non-port variables are
// rejected.
//
// We catch the static shapes:
//
//   - any-from on a port instance declared without an array
//     dimension (NegSem_220302_004 / 005 family);
//   - any-from on a variable whose declared type is not a port
//     type at all (NegSem_220302_020 - `var anytype p`).
//
// Multi-component port arrays accessed via `compRef.p` are out of
// scope for this check; we only diagnose the bare-instance form.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAnyFromPortRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	portTypes := collectPortTypes(mod)
	if len(portTypes) == 0 {
		return diags
	}
	compPortInfo := collectComponentPortInfo(mod)
	parents := collectComponentParents(mod)
	compPortInfo = flattenComponentPortInfo(compPortInfo, parents)
	compMemberVarTypes := collectComponentMemberVarTypes(mod)

	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		comp := ""
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			comp = syntax.Name(fn.RunsOn.Comp)
		}
		ports := compPortInfo[comp]
		if ports == nil {
			continue
		}
		localVars := collectFuncBodyVarTypes(fn.Body)
		// Member vars on the runs-on component are visible to
		// the function body. We collect them so a definition
		// like `var anytype p;` on the component body is seen
		// when the function does `any from p.getcall`.
		for k, v := range compMemberVarTypes[comp] {
			if _, exists := localVars[k]; !exists {
				localVars[k] = v
			}
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			fe, ok := n.(*syntax.FromExpr)
			if !ok || fe == nil || fe.X == nil || fe.KindTok == nil {
				return true
			}
			kind := fe.KindTok.Kind()
			if kind != syntax.ANYKW && kind != syntax.ALL {
				return true
			}
			sel, ok := fe.X.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			opIdent, ok := sel.Sel.(*syntax.Ident)
			if !ok || opIdent == nil {
				return true
			}
			op := opIdent.String()
			if portOpFamily(op) == "" && op != "check" {
				return true
			}
			portIdent, ok := sel.X.(*syntax.Ident)
			if !ok || portIdent == nil {
				return true
			}
			name := portIdent.String()
			if info, isPort := ports[name]; isPort {
				if !info.isArray {
					diags = append(diags, Diagnostic{
						Code:     "any-from-non-port-array",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"any/all from %s.%s: receiver must be a port array (`port T %s[N]`); single port instances are not allowed (ETSI 22.2.2)",
							name, op, name),
						Node: fe,
						Span: syntax.SpanOf(fe),
					})
				}
				return true
			}
			// Not a port instance at all. Look at the
			// var's declared type; if it isn't a known
			// port type, emit a more pointed diagnostic.
			if typ, isVar := localVars[name]; isVar {
				if _, isPortType := portTypes[typ]; !isPortType {
					diags = append(diags, Diagnostic{
						Code:     "any-from-non-port-ref",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"any/all from %s.%s: %s is of type %q, not a port type (ETSI 22.3.1 n))",
							name, op, name, typ),
						Node: fe,
						Span: syntax.SpanOf(fe),
					})
				}
			}
			return true
		})
	}
	return diags
}

// componentPortInfo augments collectComponentPorts with the per-
// instance array status. We can't reuse collectComponentPorts'
// `map[string]string` directly because it drops the dim info.
type componentPortInfo struct {
	portType string
	isArray  bool
	dims     int
}

func collectComponentPortInfo(mod *syntax.Module) map[string]map[string]componentPortInfo {
	out := map[string]map[string]componentPortInfo{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil || ct.Body == nil {
			continue
		}
		ports := map[string]componentPortInfo{}
		for _, stmt := range ct.Body.Stmts {
			ds, ok := stmt.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.PORT {
				continue
			}
			id, ok := vd.Type.(*syntax.Ident)
			if !ok {
				continue
			}
			portType := id.String()
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				ports[dec.Name.String()] = componentPortInfo{
					portType: portType,
					isArray:  len(dec.ArrayDef) > 0,
					dims:     len(dec.ArrayDef),
				}
			}
		}
		out[ct.Name.String()] = ports
	}
	return out
}

// collectComponentMemberVarTypes returns per-component-type a map
// of non-port member-declaration name -> declared-type-name. Used
// to recognise the `var <T> p;` shape in NegSem_220302_020 where
// the `any from p.getcall` receiver is a member variable that's
// not a port at all.
func collectComponentMemberVarTypes(mod *syntax.Module) map[string]map[string]string {
	out := map[string]map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
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
			if vd.KindTok.Kind() == syntax.PORT {
				continue
			}
			typeName := identName(vd.Type)
			if typeName == "" {
				continue
			}
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

// collectFuncBodyVarTypes returns name -> declared-type-name for
// every var/const declaration inside body. The check uses it to
// classify "is the receiver of any-from a port-typed variable?"
// at static-analysis time without a real type resolver.
func collectFuncBodyVarTypes(body syntax.Node) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := identName(vd.Type)
		if typeName == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = typeName
		}
		return true
	})
	return out
}

// flattenComponentPortInfo mirrors flattenComponentPorts for the
// info map. Inherited port instances are merged in; the local
// declaration wins for overlap.
func flattenComponentPortInfo(info map[string]map[string]componentPortInfo, parents map[string][]string) map[string]map[string]componentPortInfo {
	out := map[string]map[string]componentPortInfo{}
	for ct := range info {
		merged := map[string]componentPortInfo{}
		visited := map[string]bool{}
		var walk func(name string)
		walk = func(name string) {
			if visited[name] {
				return
			}
			visited[name] = true
			for _, p := range parents[name] {
				walk(p)
			}
			for k, v := range info[name] {
				if _, ok := merged[k]; !ok {
					merged[k] = v
				}
			}
		}
		walk(ct)
		out[ct] = merged
	}
	return out
}
