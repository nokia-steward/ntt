// map_topology_rules.go enforces ETSI ES 201 873-1 clause
// 9.1 (Figure 7): a component port that has already appeared
// as the component-side argument of `connect(...)` may not
// later appear as the component-side argument of
// `map(..., system:...)`, and vice-versa. The same port
// instance cannot be both connected to another component
// and mapped to the system interface within the same
// testcase body, unless a `disconnect` / `unmap` in between
// released it (21.1.2).
//
// We deliberately do NOT enforce the older "one system port
// maps to at most one component port" rule (NegSem_0902_001
// / NegSem_0902_004): the suite's own positive
// Sem_0901_008 and Syn_0902_001 tests demonstrate that the
// language now allows a single TSI port to multiplex to
// several component ports (Figure 6 scheme h).
//
// The analysis is purely textual / per-block: each testcase
// body is walked once and the connect / map operations are
// recorded in the order they appear. Any subsequent
// operation that violates the recorded state emits a
// diagnostic. Cross-function and cross-component flow is
// intentionally not modelled.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkMapTopologyRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.KindTok == nil {
			continue
		}
		if fn.KindTok.Kind() != syntax.TESTCASE {
			continue
		}
		diags = append(diags, walkMapTopology(fn.Body)...)
	}
	return diags
}

// walkMapTopology scans the testcase body once and tracks the
// set of system-side ports that have been mapped and component
// ports that have been connect / map'd so far. Sequential
// statements are inspected in order; nested control flow is
// joined optimistically (we walk recursively but reset state
// inside loops to avoid false positives on iterating maps).
func walkMapTopology(body *syntax.BlockStmt) []Diagnostic {
	var diags []Diagnostic
	connectedCompPorts := map[string]map[string]bool{}
	mappedCompPorts := map[string]bool{}
	var walk func(stmt syntax.Stmt)
	walk = func(stmt syntax.Stmt) {
		if stmt == nil {
			return
		}
		switch s := stmt.(type) {
		case *syntax.BlockStmt:
			if s != nil {
				for _, sub := range s.Stmts {
					walk(sub)
				}
			}
		case *syntax.IfStmt:
			if s != nil {
				walk(s.Then)
				walk(s.Else)
			}
		case *syntax.ForStmt, *syntax.ForRangeStmt,
			*syntax.WhileStmt, *syntax.DoWhileStmt:
			// loops may map/connect on each iteration; we
			// can't soundly model that, so skip.
			return
		case *syntax.SelectStmt:
			if s != nil {
				for _, cc := range s.Body {
					walk(cc)
				}
			}
		case *syntax.AltStmt:
			if s != nil {
				walk(s.Body)
			}
		case *syntax.CommClause:
			if s != nil {
				walk(s.Body)
			}
		case *syntax.ExprStmt:
			if s == nil || s.Expr == nil {
				return
			}
			diag := checkMapTopologyExpr(s.Expr, connectedCompPorts, mappedCompPorts)
			if diag != nil {
				diags = append(diags, *diag)
			}
		}
	}
	walk(body)
	return diags
}

// checkMapTopologyExpr applies one statement-level operation to the
// topology state. connectedCompPorts maps each connected component
// port to its peers; mappedCompPorts holds the component ports
// mapped to the system. `disconnect` and `unmap` release the ports
// they name (21.1.2), so a later `map` / `connect` of them is legal.
func checkMapTopologyExpr(
	expr syntax.Expr,
	connectedCompPorts map[string]map[string]bool,
	mappedCompPorts map[string]bool,
) *Diagnostic {
	var args []syntax.Expr
	var id *syntax.Ident
	switch x := expr.(type) {
	case *syntax.Ident:
		// `disconnect;` / `unmap;` without parentheses.
		id = x
	case *syntax.CallExpr:
		id, _ = x.Fun.(*syntax.Ident)
		if x.Args != nil {
			args = x.Args.List
		}
	}
	if id == nil {
		return nil
	}
	name := id.String()
	switch name {
	case "disconnect":
		releaseConnections(args, connectedCompPorts)
		return nil
	case "unmap":
		releaseMappings(args, mappedCompPorts)
		return nil
	case "connect", "map":
	default:
		return nil
	}
	ce, ok := expr.(*syntax.CallExpr)
	if !ok || len(args) < 2 {
		return nil
	}
	leftComp, leftPort := portRefParts(args[0])
	rightComp, rightPort := portRefParts(args[1])
	if leftComp == "" || rightComp == "" {
		return nil
	}
	leftComp, rightComp = topologyComp(leftComp), topologyComp(rightComp)
	switch name {
	case "connect":
		// Both sides are component ports. Mark them as
		// connected and flag any side that was already
		// mapped to the system interface.
		left := leftComp + ":" + leftPort
		right := rightComp + ":" + rightPort
		for _, key := range []string{left, right} {
			if mappedCompPorts[key] {
				return &Diagnostic{
					Code:     "connect-on-mapped-port",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`connect` references port %q which was previously mapped to a system port; the same port cannot be both connected and mapped (ETSI 9.1 Figure 7)",
						key),
					Node: ce,
					Span: syntax.SpanOf(ce),
				}
			}
		}
		addPeer(connectedCompPorts, left, right)
		addPeer(connectedCompPorts, right, left)
	case "map":
		// Determine the component-side port (the side not
		// labelled `system`). When neither side is `system`
		// we cannot localise the topology to a TSI mapping,
		// so we skip.
		var compKey string
		switch {
		case leftComp == "system":
			compKey = rightComp + ":" + rightPort
		case rightComp == "system":
			compKey = leftComp + ":" + leftPort
		default:
			return nil
		}
		if len(connectedCompPorts[compKey]) > 0 {
			return &Diagnostic{
				Code:     "map-on-connected-port",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`map` references component port %q which was previously connected; the same port cannot be both connected and mapped (ETSI 9.1 Figure 7)",
					compKey),
				Node: ce,
				Span: syntax.SpanOf(ce),
			}
		}
		mappedCompPorts[compKey] = true
	}
	return nil
}

func addPeer(connected map[string]map[string]bool, port, peer string) {
	if connected[port] == nil {
		connected[port] = map[string]bool{}
	}
	connected[port][peer] = true
}

// topologyComp names the component of a port reference; in a
// testcase body `self` is the mtc.
func topologyComp(comp string) string {
	if comp == "self" {
		return "mtc"
	}
	return comp
}

// portRefMatches reports whether the `<comp>:<port>` reference
// (`all component` and `all port` allowed) denotes key.
func portRefMatches(comp, port, key string) bool {
	kc, kp, _ := strings.Cut(key, ":")
	return (comp == "all component" || comp == kc) &&
		(port == "all port" || port == kp)
}

// releaseConnections applies `disconnect(args)` (21.1.2): without
// arguments it releases the ports of the invoking component, with
// one reference every connection of the ports it denotes, with two
// the connections between them. A shape we cannot read releases
// everything, so it never leads to a false report.
func releaseConnections(args []syntax.Expr, connected map[string]map[string]bool) {
	var refs [][2]string
	for _, a := range args {
		comp, port := portRefParts(a)
		if comp == "" {
			clear(connected)
			return
		}
		refs = append(refs, [2]string{topologyComp(comp), port})
	}
	if len(refs) == 0 {
		refs = [][2]string{{"mtc", "all port"}}
	}
	if len(refs) > 2 {
		clear(connected)
		return
	}
	matches := func(ref [2]string, key string) bool {
		return portRefMatches(ref[0], ref[1], key)
	}
	for key, peers := range connected {
		for peer := range peers {
			var released bool
			if len(refs) == 1 {
				released = matches(refs[0], key) || matches(refs[0], peer)
			} else {
				released = matches(refs[0], key) && matches(refs[1], peer) ||
					matches(refs[1], key) && matches(refs[0], peer)
			}
			if released {
				delete(peers, peer)
			}
		}
	}
	for key, peers := range connected {
		if len(peers) == 0 {
			delete(connected, key)
		}
	}
}

// releaseMappings applies `unmap(args)` (21.1.2) to the mapped
// component ports. A reference to a system port, or a shape we
// cannot read, releases everything.
func releaseMappings(args []syntax.Expr, mapped map[string]bool) {
	var refs [][2]string
	for _, a := range args {
		comp, port := portRefParts(a)
		if comp == "" || comp == "system" {
			continue
		}
		refs = append(refs, [2]string{topologyComp(comp), port})
	}
	switch {
	case len(args) == 0:
		refs = [][2]string{{"mtc", "all port"}}
	case len(refs) != 1:
		clear(mapped)
		return
	}
	for key := range mapped {
		if portRefMatches(refs[0][0], refs[0][1], key) {
			delete(mapped, key)
		}
	}
}

// portRefParts splits a `<comp>:<port>` BinaryExpr into the two
// identifier names. Returns empty strings for any other shape.
func portRefParts(expr syntax.Expr) (string, string) {
	be, ok := expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return "", ""
	}
	left, ok := be.X.(*syntax.Ident)
	if !ok || left == nil {
		return "", ""
	}
	right, ok := be.Y.(*syntax.Ident)
	if !ok || right == nil {
		return "", ""
	}
	return left.String(), right.String()
}
