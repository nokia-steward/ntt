package interpreter

// connections.go models the test-configuration topology (ETSI ES 201
// 873-1 clause 21.1): `connect`/`disconnect` between component ports and
// `map`/`unmap` to the system. The graph lives on the TestcaseExec and
// is queried by `p.checkstate("Connected"|"Mapped"|...)` so a port's
// state reflects the connect/map operations actually performed rather
// than a constant.

import (
	"strings"

	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// Sentinel component ids for endpoints that are not a concrete PTC.
const (
	mtcEndpointID    int64 = 0  // the MTC / testcase main thread
	systemEndpointID int64 = -2 // the `system` side of a map
)

// resolvePortEndpoint turns a `connect`/`map` argument into a
// (component,port) endpoint. The argument is either `comp:port`
// (a BinaryExpr whose left names the component and right the port) or a
// bare `port` identifier referring to the current component's port.
// The second return reports whether the component side is `system`.
func resolvePortEndpoint(e syntax.Expr, env runtime.Scope) (runtime.PortEndpoint, bool, bool) {
	exec := runtime.FindTestcaseExec(env)
	var compExpr syntax.Expr
	var portName string
	switch v := e.(type) {
	case *syntax.BinaryExpr:
		compExpr = v.X
		portName = portRefName(v.Y)
	case *syntax.Ident:
		portName = v.String()
	default:
		return runtime.PortEndpoint{}, false, false
	}
	if portName == "" {
		return runtime.PortEndpoint{}, false, false
	}
	compID, isSystem := resolveComponentID(compExpr, env, exec)
	return runtime.PortEndpoint{Comp: compID, Port: portName}, isSystem, true
}

// portRefName extracts the port name from a port reference, accepting
// both a bare port (`p`) and an array element (`p[i]`).
func portRefName(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.Ident:
		return v.String()
	case *syntax.IndexExpr:
		return portRefName(v.X)
	case *syntax.SelectorExpr:
		if id, ok := v.Sel.(*syntax.Ident); ok {
			return id.String()
		}
	}
	return ""
}

// resolvePortName resolves a port reference to its (possibly aliased)
// instance name and reports whether it is genuinely a port. A bare
// port and an array element keep their syntactic instance name; a
// formal port parameter is bound to a PortRef carrying the originating
// caller's instance name, so `p_port.stop` resolves to `p`. The isPort
// flag (the base resolving to a PortRef) tells a port `.stop`/`.start`/
// `.halt` apart from the component and timer ops sharing those names.
func resolvePortName(e syntax.Expr, env runtime.Scope) (name string, isPort bool) {
	full, ok := portExprName(e, env)
	if !ok {
		full = portRefName(e)
	}
	base := portRefName(e)
	if base == "" {
		return full, false
	}
	if v, ok := env.Get(base); ok {
		if pr, ok := forceThunk(v).(*runtime.PortRef); ok {
			if full == base {
				return pr.Name, true
			}
			// Array element on an aliased base (`pa[i]` where pa is a
			// formal): swap the base prefix for the aliased name.
			return pr.Name + full[len(base):], true
		}
	}
	return full, false
}

// resolveComponentID resolves the component side of an endpoint to a
// stable id and reports whether it is the `system` component. A nil
// expression (bare port form) and `self` resolve to the running
// component; `system` to the system sentinel; every other reference
// (`mtc`, `v_ptc`, ...) is evaluated and its ComponentRef id used so
// the id matches what CurrentComponent reports inside that component's
// body (the MTC and PTCs are real ComponentRefs with stable ids).
func resolveComponentID(e syntax.Expr, env runtime.Scope, exec *runtime.TestcaseExec) (int64, bool) {
	if e == nil {
		return currentComponentID(exec), false
	}
	if id, ok := e.(*syntax.Ident); ok {
		switch id.String() {
		case "self":
			return currentComponentID(exec), false
		case "system":
			return systemEndpointID, true
		}
	}
	if v := eval(e, env); !runtime.IsError(v) {
		if ref, ok := v.(*runtime.ComponentRef); ok && ref != nil {
			return ref.ID, false
		}
	}
	return currentComponentID(exec), false
}

func currentComponentID(exec *runtime.TestcaseExec) int64 {
	if exec != nil {
		if c := exec.CurrentComponent(); c != nil {
			return c.ID
		}
	}
	return mtcEndpointID
}

// evalConnectOp implements `connect`/`disconnect` (ETSI 21.1.1/21.1.2)
// by mutating the testcase connection graph. Returns Undefined so the
// statement behaves like the previous no-op for callers that ignore the
// result.
func evalConnectOp(op string, n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || n.Args == nil {
		return runtime.Undefined
	}
	args := n.Args.List
	switch op {
	case "connect":
		if len(args) < 2 {
			return runtime.Undefined
		}
		a, _, okA := resolvePortEndpoint(args[0], env)
		b, _, okB := resolvePortEndpoint(args[1], env)
		if okA && okB {
			exec.ConnectPorts(a, b)
		}
	case "disconnect":
		if len(args) == 0 {
			// `disconnect;` (no args) tears down every connection
			// of the running component (ETSI 21.1.2).
			exec.DisconnectComponent(currentComponentID(exec))
			return runtime.Undefined
		}
		a, _, okA := resolvePortEndpoint(args[0], env)
		if !okA {
			return runtime.Undefined
		}
		if len(args) >= 2 {
			if b, _, okB := resolvePortEndpoint(args[1], env); okB {
				exec.DisconnectPorts(a, b)
			}
		} else if isAllComponentArg(args[0]) {
			// `disconnect(all component:all port)` - release every
			// connection in the configuration.
			exec.ClearAllConnections()
		} else if isAllPortRef(a.Port) {
			// `disconnect(c:all port)` - drop every connection of
			// every port on component c.
			exec.DisconnectComponent(a.Comp)
		} else {
			exec.DisconnectAll(a)
		}
	}
	return runtime.Undefined
}

// recordPortMapState updates the mapping graph for a port-form
// `map`/`unmap` (`map(self:p, system:p)`). Only the component-side
// endpoints (not the `system` side) are recorded, so a subsequent
// `p.checkstate("Mapped")` on that component answers true. It reports
// whether the call looked like the port form (so the caller can avoid
// the data-structure `unmap(M, k)` interpretation).
func recordPortMapState(op string, n *syntax.CallExpr, env runtime.Scope) bool {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || n.Args == nil || len(n.Args.List) == 0 {
		return false
	}
	if !isPortMapForm(n.Args.List[0]) {
		return false
	}
	args := n.Args.List
	a, _, okA := resolvePortEndpoint(args[0], env)
	if !okA {
		return false
	}
	if op == "map" {
		if len(args) >= 2 {
			if b, _, okB := resolvePortEndpoint(args[1], env); okB {
				exec.MapPorts(a, b)
			}
		}
		return true
	}
	// unmap: two-argument form removes the specific edge; the
	// one-argument form (`unmap(system:p)` / `unmap(self:p)`) removes
	// every edge touching that side; `all component:all port` clears
	// everything; `c:all port` clears one component.
	if len(args) >= 2 {
		if b, _, okB := resolvePortEndpoint(args[1], env); okB {
			exec.UnmapPorts(a, b)
		}
	} else if isAllComponentArg(args[0]) {
		exec.ClearAllMappings()
	} else if isAllPortRef(a.Port) {
		exec.UnmapComponent(a.Comp)
	} else {
		exec.UnmapAll(a)
	}
	return true
}

// isAllPortRef reports whether a port name denotes the `all port`
// wildcard. The lexer glues `all port` into a single identifier, so we
// also accept a bare `all`.
func isAllPortRef(port string) bool {
	return port == "all port" || port == "all"
}

// isAllComponentArg reports whether a connect/disconnect/map/unmap
// argument is the `all component:all port` wildcard that releases every
// connection / mapping in the configuration (ETSI 21.1.2).
func isAllComponentArg(e syntax.Expr) bool {
	be, ok := e.(*syntax.BinaryExpr)
	if !ok {
		return false
	}
	id, ok := be.X.(*syntax.Ident)
	if !ok {
		return false
	}
	return id.String() == "all component" || id.String() == "all"
}

// isPortMapForm reports whether a map/unmap first argument is a
// `comp:port` endpoint (the configuration form) rather than a plain
// identifier (the `unmap(M, k)` data-structure form).
func isPortMapForm(e syntax.Expr) bool {
	_, ok := e.(*syntax.BinaryExpr)
	return ok
}

// evalPortCheckstate implements `p.checkstate(state)` (ETSI 21.1.3)
// against the real connection/mapping graph. The affirmative states
// ("Connected", "Mapped") answer from the running component's endpoint;
// the matching negative states are their complement. "Started" and
// other port-lifecycle states keep the optimistic loopback answer.
func evalPortCheckstate(n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	if n.Args == nil || len(n.Args.List) == 0 {
		return runtime.NewBool(false)
	}
	v := eval(n.Args.List[0], env)
	if runtime.IsError(v) {
		return v
	}
	s, ok := v.(*runtime.String)
	if !ok {
		return runtime.NewBool(false)
	}
	state := strings.ToLower(s.String())

	port := ""
	if sel, ok := n.Fun.(*syntax.SelectorExpr); ok {
		if pn, _ := resolvePortName(sel.X, env); pn != "" {
			port = pn
		} else {
			port = portRefName(sel.X)
		}
	}
	exec := runtime.FindTestcaseExec(env)
	ep := runtime.PortEndpoint{Comp: currentComponentID(exec), Port: port}

	// Explicit p.start / p.stop / p.halt state (ETSI 22.1). A port
	// never touched by a lifecycle op keeps the optimistic default
	// (started, not stopped, not halted).
	lifecycle := "started"
	if exec != nil && port != "" {
		if s, ok := exec.PortLifecycle(ep); ok {
			lifecycle = s
		}
	}

	switch state {
	case "connected":
		if exec != nil && port != "" {
			return runtime.NewBool(exec.IsConnected(ep))
		}
		return runtime.NewBool(false)
	case "unconnected":
		if exec != nil && port != "" {
			return runtime.NewBool(!exec.IsConnected(ep))
		}
		return runtime.NewBool(true)
	case "mapped":
		if exec != nil && port != "" {
			return runtime.NewBool(exec.IsMapped(ep))
		}
		return runtime.NewBool(false)
	case "unmapped":
		if exec != nil && port != "" {
			return runtime.NewBool(!exec.IsMapped(ep))
		}
		return runtime.NewBool(true)
	case "started":
		return runtime.NewBool(lifecycle == "started")
	case "stopped":
		return runtime.NewBool(lifecycle == "stopped")
	case "halted":
		return runtime.NewBool(lifecycle == "halted")
	case "linked":
		return runtime.NewBool(true)
	case "unstarted", "unlinked":
		return runtime.NewBool(lifecycle != "started")
	}
	return runtime.NewBool(false)
}

// isPortLifecycleOp reports whether op is one of the no-argument port
// state operations (ETSI 22.1).
func isPortLifecycleOp(op string) bool {
	switch op {
	case "start", "stop", "halt":
		return true
	}
	return false
}

// applyPortLifecycle records the new operational state of a port
// instance after a p.start / p.stop / p.halt operation.
func applyPortLifecycle(op, port string, env runtime.Scope) {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || port == "" {
		return
	}
	state := "started"
	switch op {
	case "stop":
		state = "stopped"
	case "halt":
		state = "halted"
	}
	exec.SetPortLifecycle(runtime.PortEndpoint{Comp: currentComponentID(exec), Port: port}, state)
	// Forward start/stop/halt to a bound port driver that opts into the
	// PortController interface (a pure-Go or C test port). Gated on a
	// driver being present, so loopback ports are unaffected.
	if drv := exec.PortDriver(port); drv != nil {
		if c, ok := drv.(runtime.PortController); ok {
			switch op {
			case "start":
				_ = c.Start(port)
			case "stop", "halt":
				_ = c.Stop(port)
			}
		}
	}
}
