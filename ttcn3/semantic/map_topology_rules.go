// map_topology_rules.go enforces ETSI ES 201 873-1 clause
// 9.1 (Figure 7): a component port that has already appeared
// as the component-side argument of `connect(...)` may not
// later appear as the component-side argument of
// `map(..., system:...)`, and vice-versa. The same port
// instance cannot be both connected to another component
// and mapped to the system interface within the same
// testcase body.
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
	connectedCompPorts := map[string]bool{}
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

func checkMapTopologyExpr(
	expr syntax.Expr,
	connectedCompPorts, mappedCompPorts map[string]bool,
) *Diagnostic {
	ce, ok := expr.(*syntax.CallExpr)
	if !ok || ce == nil {
		return nil
	}
	id, ok := ce.Fun.(*syntax.Ident)
	if !ok || id == nil {
		return nil
	}
	name := id.String()
	if name != "connect" && name != "map" {
		return nil
	}
	if ce.Args == nil || len(ce.Args.List) < 2 {
		return nil
	}
	leftComp, leftPort := portRefParts(ce.Args.List[0])
	rightComp, rightPort := portRefParts(ce.Args.List[1])
	if leftComp == "" || rightComp == "" {
		return nil
	}
	switch name {
	case "connect":
		// Both sides are component ports. Mark them as
		// connected and flag any side that was already
		// mapped to the system interface.
		for _, key := range []string{
			leftComp + ":" + leftPort,
			rightComp + ":" + rightPort,
		} {
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
			connectedCompPorts[key] = true
		}
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
		if connectedCompPorts[compKey] {
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
