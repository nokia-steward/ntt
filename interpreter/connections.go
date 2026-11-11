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
	"github.com/nokia/ntt/runtime/tl"
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
		portName = portElementName(v.Y, env)
	case *syntax.Ident, *syntax.IndexExpr:
		portName = portElementName(v, env)
	default:
		return runtime.PortEndpoint{}, false, false
	}
	if portName == "" {
		return runtime.PortEndpoint{}, false, false
	}
	compID, isSystem := resolveComponentID(compExpr, env, exec)
	return runtime.PortEndpoint{Comp: compID, Port: portName}, isSystem, true
}

// portElementName names the port a `connect`/`map` argument refers to: an
// element of a port array by its element (`p[1]`), each element being a
// port of its own (ETSI ES 201 873-1 21.1), else the port.
func portElementName(e syntax.Expr, env runtime.Scope) string {
	if _, ok := e.(*syntax.IndexExpr); ok {
		if name, ok := portExprName(e, env); ok {
			return name
		}
	}
	return portRefName(e)
}

// connectedPeers returns the endpoints port of component comp is connected
// to. An element of a port array is connected as itself; one whose array
// was connected as a whole reaches the same element of the peer's array.
func connectedPeers(exec *runtime.TestcaseExec, comp int64, port string) []runtime.PortEndpoint {
	if peers := exec.ConnectedPeers(runtime.PortEndpoint{Comp: comp, Port: port}); len(peers) > 0 {
		return peers
	}
	base, suffix := splitPortIndex(port)
	if suffix == "" {
		return nil
	}
	peers := exec.ConnectedPeers(runtime.PortEndpoint{Comp: comp, Port: base})
	for i := range peers {
		peers[i].Port += suffix
	}
	return peers
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
			// A reference to the system component, as a parameter holds
			// it, is the system's.
			return ref.ID, ref.ID == systemEndpointID
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
		if err := uncreatedComponentError("connect", args, env); err != nil {
			return err
		}
		a, aSys, okA := resolvePortEndpoint(args[0], env)
		b, bSys, okB := resolvePortEndpoint(args[1], env)
		if okA && okB {
			if err := connectError(exec, a, b, aSys, bSys); err != nil {
				return err
			}
			exec.ConnectPorts(a, b)
			tlPortConfig(exec, n, "tliPConnect", a, b)
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
				tlPortConfig(exec, n, "tliPDisconnect", a, b)
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
			if exec.TestLogger() != nil {
				for _, b := range exec.ConnectedPeers(a) {
					tlPortConfig(exec, n, "tliPDisconnect", a, b)
				}
			}
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
// the data-structure `unmap(M, k)` interpretation), and the error of a
// map the configuration rules forbid.
func recordPortMapState(op string, n *syntax.CallExpr, env runtime.Scope) (bool, runtime.Object) {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || n.Args == nil || len(n.Args.List) == 0 {
		return false, nil
	}
	if !isPortMapForm(n.Args.List[0]) {
		return false, nil
	}
	args := n.Args.List
	if op == "map" {
		if err := uncreatedComponentError("map", args, env); err != nil {
			return true, err
		}
	}
	a, aSys, okA := resolvePortEndpoint(args[0], env)
	if !okA {
		return false, nil
	}
	if op == "map" {
		if len(args) >= 2 {
			if b, bSys, okB := resolvePortEndpoint(args[1], env); okB {
				if err := mapError(exec, a, b, aSys, bSys); err != nil {
					return true, err
				}
				exec.MapPorts(a, b)
				tlPortConfig(exec, n, "tliPMap", a, b)
			}
		}
		return true, nil
	}
	// unmap: two-argument form removes the specific edge; the
	// one-argument form (`unmap(system:p)` / `unmap(self:p)`) removes
	// every edge touching that side; `all component:all port` clears
	// everything; `c:all port` clears one component.
	if len(args) >= 2 {
		if b, _, okB := resolvePortEndpoint(args[1], env); okB {
			exec.UnmapPorts(a, b)
			tlPortConfig(exec, n, "tliPUnmap", a, b)
		}
	} else if isAllComponentArg(args[0]) {
		exec.ClearAllMappings()
	} else if isAllPortRef(a.Port) {
		exec.UnmapComponent(a.Comp)
	} else {
		exec.UnmapAll(a)
	}
	return true, nil
}

// uncreatedComponentError is the error of op on a port of a component
// that is not created: its reference null or uninitialised (ETSI 21.1.1),
// or nil.
func uncreatedComponentError(op string, args []syntax.Expr, env runtime.Scope) runtime.Object {
	for _, a := range args {
		be, ok := a.(*syntax.BinaryExpr)
		if !ok || be.X == nil {
			continue
		}
		switch x := be.X.(type) {
		case *syntax.Ident:
			switch x.String() {
			case "self", "system", "mtc", "all component", "all":
				continue
			}
		case *syntax.SelectorExpr, *syntax.IndexExpr:
		default:
			// A function instance (ObjectReference) is not evaluated
			// twice.
			continue
		}
		v := eval(be.X, env)
		if v == runtime.Null || v == runtime.Undefined {
			return runtime.Errorf("%s of a port of %s, which refers to no component created (ETSI 21.1.1)", op, syntax.Name(be.X))
		}
		if ref, ok := v.(*runtime.ComponentRef); ok && componentEnded(ref) {
			return runtime.Errorf("%s of a port of %s, a component that has ended (ETSI 21.1.1, 21.3.3, 21.3.4)", op, syntax.Name(be.X))
		}
	}
	return nil
}

// componentEnded reports whether ref is a component that is no more:
// killed, or one not alive-type whose behaviour is done (ETSI 21.3.3,
// 21.3.4). Its connections are released.
func componentEnded(ref *runtime.ComponentRef) bool {
	if ref == nil || ref.ID == mtcEndpointID || ref.ID == systemEndpointID {
		return false
	}
	return !ref.IsAlive() || ref.IsDone() && !ref.AliveModifier
}

// livePeers returns what endpoint e is connected to (map false) or mapped
// to (map true), but the ports of components that have ended.
func livePeers(exec *runtime.TestcaseExec, e runtime.PortEndpoint, mapped bool) []runtime.PortEndpoint {
	peers := exec.ConnectedPeers(e)
	if mapped {
		peers = exec.MappedPeers(e)
	}
	ended := map[int64]bool{}
	for _, ref := range exec.AllComponents() {
		if componentEnded(ref) {
			ended[ref.ID] = true
		}
	}
	if len(ended) == 0 {
		return peers
	}
	out := peers[:0:0]
	for _, p := range peers {
		if !ended[p.Comp] {
			out = append(out, p)
		}
	}
	return out
}

// connectError is the error of connecting a and b that clauses 9.1 a and
// 21.1.1 b forbid, or nil. Connecting ports already connected to each
// other does nothing (figure F.6).
func connectError(exec *runtime.TestcaseExec, a, b runtime.PortEndpoint, aSys, bSys bool) runtime.Object {
	if aSys || bSys {
		return runtime.Errorf("connect of a port of the test system interface; it is mapped (ETSI 21.1.1 b, 9.1 a)")
	}
	for _, p := range livePeers(exec, a, false) {
		if p == b {
			return nil
		}
	}
	for _, e := range []runtime.PortEndpoint{a, b} {
		if len(livePeers(exec, e, true)) > 0 {
			return runtime.Errorf("connect of port %s, which is mapped (ETSI 9.1 a)", e.Port)
		}
	}
	for _, pair := range [][2]runtime.PortEndpoint{{a, b}, {b, a}} {
		for _, p := range livePeers(exec, pair[0], false) {
			if p.Comp == pair[1].Comp && p != pair[1] {
				return runtime.Errorf("connect of port %s to a second port of one component, %s and %s (ETSI 9.1 a)", pair[0].Port, p.Port, pair[1].Port)
			}
		}
	}
	return nil
}

// mapError is the error of mapping a and b that clauses 9.1 a and 21.1.1
// c forbid, or nil: one of them is a port of the test system interface,
// the other a component's. Mapping ports already mapped to each other
// does nothing (figure F.6).
func mapError(exec *runtime.TestcaseExec, a, b runtime.PortEndpoint, aSys, bSys bool) runtime.Object {
	if aSys == bSys {
		return runtime.Errorf("map of two ports of which one is to be of the test system interface (ETSI 21.1.1 c)")
	}
	c, s := a, b
	if aSys {
		c, s = b, a
	}
	if peers := livePeers(exec, c, true); len(peers) > 0 {
		for _, p := range peers {
			if p == s {
				return nil
			}
		}
		return runtime.Errorf("map of port %s, which is mapped already (ETSI 9.1 a)", c.Port)
	}
	if len(livePeers(exec, c, false)) > 0 {
		return runtime.Errorf("map of port %s, which is connected (ETSI 9.1 a)", c.Port)
	}
	for _, p := range livePeers(exec, s, true) {
		if p.Comp == c.Comp {
			return runtime.Errorf("map of system port %s to a second port of one component, %s and %s (ETSI 9.1 a)", s.Port, p.Port, c.Port)
		}
	}
	return nil
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

// evalPortCheckstate implements `p.checkstate(state)`, `any
// port.checkstate(state)` and `all port.checkstate(state)` (ETSI
// 22.5.5) against the running component's ports. The state is one of
// six charstrings, compared exactly; any other value is an error.
func evalPortCheckstate(n *syntax.CallExpr, env runtime.Scope) runtime.Object {
	if n.Args == nil || len(n.Args.List) != 1 {
		return runtime.Errorf("checkstate takes one argument, the state (ETSI 22.5.5)")
	}
	v := eval(n.Args.List[0], env)
	if runtime.IsError(v) {
		return v
	}
	s, ok := v.(*runtime.String)
	if !ok || !isCheckstateState(s.String()) {
		return runtime.Errorf("checkstate: invalid state %s, want one of Started, Halted, Stopped, Connected, Mapped, Linked (ETSI 22.5.5)", v.Inspect())
	}
	state := s.String()
	exec := runtime.FindTestcaseExec(env)

	sel, _ := n.Fun.(*syntax.SelectorExpr)
	if sel == nil {
		return runtime.NewBool(portInState(exec, "", state))
	}
	// any port: at least one of the component's ports is in the state;
	// all port: every one is.
	switch target := syntax.Name(sel.X); target {
	case "any port", "all port":
		all := target == "all port"
		for _, port := range currentComponentPorts(env) {
			if portInState(exec, port, state) != all {
				return runtime.NewBool(!all)
			}
		}
		return runtime.NewBool(all)
	}
	port, _ := resolvePortName(sel.X, env)
	if port == "" {
		port = portRefName(sel.X)
	}
	return runtime.NewBool(portInState(exec, port, state))
}

// currentComponentPorts are the names of the running component's ports,
// declared by its type or inherited.
func currentComponentPorts(env runtime.Scope) []string {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil {
		return nil
	}
	comp := exec.CurrentComponent()
	if comp == nil {
		return nil
	}
	return componentPortNames(runtime.RootScope(env), comp.Module, comp.TypeName)
}

// isCheckstateState reports whether s is a state checkstate takes
// (ETSI 22.5.5 a).
func isCheckstateState(s string) bool {
	switch s {
	case "Started", "Halted", "Stopped", "Connected", "Mapped", "Linked":
		return true
	}
	return false
}

// portInState reports whether the running component's port is in
// state. Started, Stopped and Halted are the port's operational state
// (ETSI F.3): a port no lifecycle operation touched is started.
// Connected and Mapped follow connect/disconnect and map/unmap (21.1),
// and Linked is either.
func portInState(exec *runtime.TestcaseExec, port, state string) bool {
	known := exec != nil && port != ""
	ep := runtime.PortEndpoint{Comp: currentComponentID(exec), Port: port}
	switch state {
	case "Started", "Stopped", "Halted":
		s := runtime.PortStarted
		if known {
			s = exec.PortStateOf(exec.PortKey(port))
		}
		return portStateName(s) == state
	case "Connected":
		return known && exec.IsConnected(ep)
	case "Mapped":
		return known && exec.IsMapped(ep)
	case "Linked":
		return known && (exec.IsConnected(ep) || exec.IsMapped(ep))
	}
	return false
}

// portStateName is the checkstate name of a port state (ETSI 22.5.5).
func portStateName(s runtime.PortState) string {
	switch s {
	case runtime.PortStopped:
		return "Stopped"
	case runtime.PortHalted:
		return "Halted"
	}
	return "Started"
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
func applyPortLifecycle(n syntax.Node, op, port string, env runtime.Scope) {
	exec := runtime.FindTestcaseExec(env)
	if exec == nil || port == "" {
		return
	}
	key := exec.PortKey(port)
	switch op {
	case "start":
		exec.StartPort(key)
	case "stop":
		exec.StopPort(key)
	case "halt":
		exec.HaltPort(key)
	}
	if exec.TestLogger() != nil {
		tlEmitLazy(exec, siteOf(n), "tliP"+strings.ToUpper(op[:1])+op[1:], func() []tl.Arg {
			return []tl.Arg{tlArg("port", tlOwnPort(exec, port).Content()), tlArg("stat", tl.String(tlPortStatus(exec, port)))}
		})
	}
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
