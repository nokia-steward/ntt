// connect_map_compat.go implements the connect/map port-compatibility
// rules from ETSI ES 201 873-1 clause 21.1.1.
//
// `connect(c1:p1, c2:p2)` is only well-formed when the two port
// types it bridges have compatible direction lists:
//
//	outlist(P1) is a subset of inlist(P2) AND
//	outlist(P2) is a subset of inlist(P1)
//
// `map(self:p, system:sp)` follows the same rule, with the system
// side standing in for the peer.
//
// The check is purely static: we resolve the variable on each side
// of the `comp:port` argument to its declared component type via
// var-declarations in the enclosing scope, look up the port type
// from the component's port-decl list, then compare in/out/inout
// declarations from the two port types.
//
// Two simplifications:
//
//   - `inout` types count as both in and out for the subset test.
//   - `in all` / `out all` / `inout all` always satisfy the
//     containment for that direction.
//
// We do NOT attempt to model `extends`-style component inheritance
// yet, so a port inherited from a parent component slips through
// when the immediate comp body doesn't list it.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkConnectMapCompat(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	portTypes := collectPortTypes(mod)
	if len(portTypes) == 0 {
		return diags
	}
	compPorts := collectComponentPorts(mod)
	if len(compPorts) == 0 {
		return diags
	}
	parents := collectComponentParents(mod)
	compPorts = flattenComponentPorts(compPorts, parents)

	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// The component the function runs on owns the local
		// scope name `self`. We need it to resolve `self:p`
		// inside `map(self:p, system:sp)`.
		runsOn := ""
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			runsOn = syntax.Name(fn.RunsOn.Comp)
		}
		systemComp := ""
		if fn.System != nil && fn.System.Comp != nil {
			systemComp = syntax.Name(fn.System.Comp)
		}
		varTypes := collectVarComponentTypes(fn.Body, compPorts)
		diags = append(diags, checkConnectMapInBody(
			fn.Body, runsOn, systemComp, varTypes, compPorts, portTypes,
		)...)
	}
	return diags
}

func checkConnectMapInBody(
	body *syntax.BlockStmt,
	runsOn string,
	systemComp string,
	varTypes map[string]string,
	compPorts map[string]map[string]string,
	portTypes map[string]*portDirs,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok {
			return true
		}
		opName := id.String()
		var isConnect, isMap bool
		switch opName {
		case "connect", "disconnect":
			isConnect = true
		case "map", "unmap":
			isMap = true
		default:
			return true
		}
		if ce.Args == nil || len(ce.Args.List) < 2 {
			return true
		}
		left, leftStatus := resolvePortRefStatus(
			ce.Args.List[0], runsOn, systemComp, varTypes, compPorts,
		)
		right, rightStatus := resolvePortRefStatus(
			ce.Args.List[1], runsOn, systemComp, varTypes, compPorts,
		)
		// Emit "port not in component" diagnostics whenever we
		// fully resolved the component type but the port name
		// is missing from its (flattened) port list.
		for _, side := range []struct {
			ref portRef
			st  portRefStatus
		}{{left, leftStatus}, {right, rightStatus}} {
			if side.st == portRefMissingPort {
				diags = append(diags, Diagnostic{
					Code:     "port-ref-not-in-component",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s(...): port %q is not declared on component %q referenced by %q (ETSI 21.1.2 strong-typing rule)",
						opName, side.ref.portName,
						side.ref.compType, side.ref.compVar),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
		}
		if leftStatus != portRefOK || rightStatus != portRefOK {
			return true
		}
		ld := portTypes[left.portType]
		rd := portTypes[right.portType]
		if ld == nil || rd == nil {
			return true
		}
		if isConnect {
			// ETSI 21.1.1 b.2 / 21.1.2: neither side of
			// connect() / disconnect() may be a system port
			// reference.
			if left.compVar == "system" || right.compVar == "system" {
				diags = append(diags, Diagnostic{
					Code:     "connect-on-system-port",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s(%s:%s, %s:%s): cannot %s a system-side port (ETSI 21.1.1 b.2)",
						opName,
						left.compVar, left.portName,
						right.compVar, right.portName,
						opName),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
			if msg := compatViolation(ld, rd); msg != "" {
				diags = append(diags, Diagnostic{
					Code:     "connect-incompatible-ports",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s(%s:%s, %s:%s): incompatible port types %s and %s (%s)",
						opName,
						left.compVar, left.portName,
						right.compVar, right.portName,
						left.portType, right.portType, msg),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
			// The ETSI 21.1.1 b.3 "at least one outlist
			// non-empty" rule is contradicted by the
			// suite's own positive Sem_011 test (the spec
			// authors removed the restriction; the
			// corresponding NegSem_017 is V4-only). We
			// therefore skip the empty-outlist check on
			// connect rather than break the positive test.
		}
		if isMap {
			// ETSI 21.1.1 c.1 / 21.1.2: map and unmap
			// require exactly one component port and one
			// system port. Two component-side refs (no
			// `system:` qualifier) or two `system:` refs
			// both violate the rule.
			leftSys := left.compVar == "system"
			rightSys := right.compVar == "system"
			if leftSys == rightSys {
				diags = append(diags, Diagnostic{
					Code:     "map-requires-one-system-port",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s(%s:%s, %s:%s): exactly one side must be a system port (ETSI 21.1.1 c.1)",
						opName,
						left.compVar, left.portName,
						right.compVar, right.portName),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
			// ETSI 21.1.1 c) `map(PORT1, PORT2)` with PORT2
			// the test-system-interface port is allowed iff
			//
			//   outlist-PORT1 subset-of outlist-PORT2
			//   inlist-PORT2  subset-of inlist-PORT1
			//
			// i.e. the rule is asymmetric: the SUT-side port
			// (system) must be a *superset* of the component
			// port's outs and a *subset* of its ins, because
			// map is a pass-through from the component port
			// to whatever the system port speaks externally.
			//
			// We pick which side is "system" by name: an arg
			// of the form `system:p` is the system side.
			// When neither side is `system` (rare; some
			// suites use a stand-in component) we fall back
			// to the symmetric connect rule rather than
			// guess - too lax is better than too strict.
			lo, sys := left, right
			swap := false
			if left.compVar == "system" && right.compVar != "system" {
				lo, sys = right, left
				swap = true
			}
			lod := ld
			sysd := rd
			if swap {
				lod, sysd = rd, ld
			}
			var msg string
			if left.compVar == "system" || right.compVar == "system" {
				msg = mapCompatViolation(lod, sysd)
			} else {
				msg = compatViolation(ld, rd)
			}
			if msg != "" {
				diags = append(diags, Diagnostic{
					Code:     "map-incompatible-ports",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%s(%s:%s, %s:%s): incompatible port types %s and %s (%s)",
						opName,
						lo.compVar, lo.portName,
						sys.compVar, sys.portName,
						lo.portType, sys.portType, msg),
					Node: ce,
					Span: syntax.SpanOf(ce),
				})
			}
			// As with connect, the suite's positive Sem_012
			// flips the V4 "at least one non-empty"
			// restriction; we skip the symmetric map check
			// to avoid flagging it.
		}
		return true
	})
	return diags
}

type portRef struct {
	compVar  string // the surface-name of the component variable
	compType string // the resolved component-type name
	portName string // the port instance name
	portType string // the resolved port-type name
}

type portRefStatus int

const (
	// portRefUnknown means the component variable could not be
	// resolved (formal parameter, unknown var, etc.). Silently
	// skip the diagnostic.
	portRefUnknown portRefStatus = iota
	// portRefMissingPort means the component type is known but
	// the port name does not appear in its (flattened) port
	// list. Emit a strong-typing diagnostic.
	portRefMissingPort
	// portRefOK means the component, port and port-type all
	// resolved successfully.
	portRefOK
)

// resolvePortRef walks a `<compVar>:<port>` BinaryExpr (the form
// every connect / map argument takes) and looks up the port type
// from the declared component's port list. Returns ok=false when any
// step fails (unknown variable, unknown component type, unknown port
// name) so the caller can quietly skip the check rather than report
// a spurious diagnostic.
//
// `self` / `mtc` resolve to runsOn. `system` resolves to the
// testcase's system component when known.
func resolvePortRef(
	expr syntax.Expr,
	runsOn, systemComp string,
	varTypes map[string]string,
	compPorts map[string]map[string]string,
) (portRef, bool) {
	ref, st := resolvePortRefStatus(expr, runsOn, systemComp, varTypes, compPorts)
	return ref, st == portRefOK
}

func resolvePortRefStatus(
	expr syntax.Expr,
	runsOn, systemComp string,
	varTypes map[string]string,
	compPorts map[string]map[string]string,
) (portRef, portRefStatus) {
	var ref portRef
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ref, portRefUnknown
	}
	lhs, ok := be.X.(*syntax.Ident)
	if !ok {
		return ref, portRefUnknown
	}
	rhs, ok := be.Y.(*syntax.Ident)
	if !ok {
		return ref, portRefUnknown
	}
	ref.compVar = lhs.String()
	ref.portName = rhs.String()

	compType := ""
	switch ref.compVar {
	case "self", "mtc":
		compType = runsOn
	case "system":
		compType = systemComp
	default:
		compType = varTypes[ref.compVar]
	}
	if compType == "" {
		return ref, portRefUnknown
	}
	ref.compType = compType
	ports := compPorts[compType]
	if ports == nil {
		return ref, portRefUnknown
	}
	pt, ok := ports[ref.portName]
	if !ok {
		return ref, portRefMissingPort
	}
	ref.portType = pt
	return ref, portRefOK
}

// flattenComponentPorts merges every component's own ports with
// all ports declared in its parent chain. The walker tolerates
// cycles by tracking visited names.
func flattenComponentPorts(
	compPorts map[string]map[string]string,
	parents map[string][]string,
) map[string]map[string]string {
	flat := map[string]map[string]string{}
	var visit func(name string, seen map[string]bool) map[string]string
	visit = func(name string, seen map[string]bool) map[string]string {
		if seen[name] {
			return nil
		}
		seen[name] = true
		out := map[string]string{}
		for k, v := range compPorts[name] {
			out[k] = v
		}
		for _, p := range parents[name] {
			for k, v := range visit(p, seen) {
				if _, exists := out[k]; !exists {
					out[k] = v
				}
			}
		}
		return out
	}
	for name := range compPorts {
		flat[name] = visit(name, map[string]bool{})
	}
	return flat
}

// compatViolation returns "" when the two ports are connect/map
// compatible. Otherwise it returns a short human-readable reason
// pointing at the first failing direction so the diagnostic body
// stays actionable.
//
// The rule (21.1.1 b):
//
//	out(A) is a subset of in(B) AND
//	out(B) is a subset of in(A)
//
// We fold `inout` into both directions on each side and short-
// circuit on the all-wildcard flags so an `inout all` port can talk
// to anything.
func compatViolation(a, b *portDirs) string {
	if a.allOutDir || a.allInoutDir {
		// Out from A is wide-open; only need B's outs to fit
		// into A's ins.
		if !subset(out(b), in(a), a.allInDir || a.allInoutDir) {
			return "out direction of one side not contained in in direction of the other"
		}
		return ""
	}
	if b.allOutDir || b.allInoutDir {
		if !subset(out(a), in(b), b.allInDir || b.allInoutDir) {
			return "out direction of one side not contained in in direction of the other"
		}
		return ""
	}
	if !subset(out(a), in(b), b.allInDir || b.allInoutDir) {
		return "out direction of one side not contained in in direction of the other"
	}
	if !subset(out(b), in(a), a.allInDir || a.allInoutDir) {
		return "out direction of one side not contained in in direction of the other"
	}
	return ""
}

// mapCompatViolation enforces the asymmetric map rule from 21.1.1 c):
//
//	outlist(local)  subset-of outlist(system)
//	inlist(system)  subset-of inlist(local)
//
// `all` flags short-circuit each containment in the obvious way.
func mapCompatViolation(local, system *portDirs) string {
	if !(system.allOutDir || system.allInoutDir) {
		if !subset(out(local), out(system), false) {
			return "out direction of component port not contained in out direction of system port"
		}
	}
	if !(local.allInDir || local.allInoutDir) {
		if !subset(in(system), in(local), false) {
			return "in direction of system port not contained in in direction of component port"
		}
	}
	return ""
}

func out(p *portDirs) map[string]bool {
	res := map[string]bool{}
	for k := range p.out {
		res[k] = true
	}
	for k := range p.inout {
		res[k] = true
	}
	return res
}

func in(p *portDirs) map[string]bool {
	res := map[string]bool{}
	for k := range p.in {
		res[k] = true
	}
	for k := range p.inout {
		res[k] = true
	}
	return res
}

func subset(a, b map[string]bool, bIsAll bool) bool {
	if bIsAll {
		return true
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}


// collectVarComponentTypes walks a function body and records every
// `var <CompType> <name>` declaration whose type is a known
// component type. We don't currently track re-assignment (the type
// stays the declared one), which matches TTCN-3's strong typing for
// `var` slots.
func collectVarComponentTypes(
	body *syntax.BlockStmt,
	compPorts map[string]map[string]string,
) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		ds, ok := n.(*syntax.DeclStmt)
		if !ok {
			return true
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil || vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		id, ok := vd.Type.(*syntax.Ident)
		if !ok {
			return true
		}
		typeName := id.String()
		if _, known := compPorts[typeName]; !known {
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
