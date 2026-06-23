// call_operation_rules.go enforces a handful of static checks on the
// `port.call(template [, CallTimerValue]) { ... }` statement defined
// by ETSI ES 201 873-1 clause 22.3.1.
//
// Two rules are implemented:
//
//   - "g) CallTimerValue shall be of type float."
//     We flag any second argument that's an integer / boolean / string
//     literal. Variable references and `nowait` pass through; if a
//     variable is bound to an integer the runtime catches it.
//
//   - "b/c) When nowait is used the call statement is non-blocking
//     and therefore must not be followed by a response-handling
//     block."
//     We flag the syntactic shape `p.call(<tmpl>, nowait) { ... }`.
//
// The rule deliberately fires only on the syntactically obvious cases
// so we don't false-positive on user-defined timer constants.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkCallOperationRules(mod *syntax.Module) []Diagnostic {
	// Pre-collect the inner ExprStmt of every CallStmt so the
	// second pass can skip them and avoid duplicate diagnostics.
	wrapped := map[syntax.Node]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if cs, ok := n.(*syntax.CallStmt); ok && cs.Stmt != nil {
			wrapped[cs.Stmt] = true
		}
		return true
	})

	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.CallStmt:
			diags = append(diags, callStmtDiags(x)...)
		case *syntax.ExprStmt:
			if wrapped[x] {
				return true
			}
			if call := exprStmtCallExpr(x); call != nil {
				diags = append(diags, callTimerValueDiags(call, nil)...)
			}
		}
		return true
	})
	return diags
}

func callStmtDiags(cs *syntax.CallStmt) []Diagnostic {
	if cs == nil || cs.Stmt == nil {
		return nil
	}
	es, ok := cs.Stmt.(*syntax.ExprStmt)
	if !ok {
		return nil
	}
	call := exprStmtCallExpr(es)
	if call == nil {
		return nil
	}
	diags := callTimerValueDiags(call, cs.Body)
	diags = append(diags, callResponseSignatureDiags(call, cs.Body)...)
	return diags
}

// callResponseSignatureDiags enforces ETSI 22.3.1 h: unqualified
// getreply and catch alternatives inside a call(SIG, ...) { ... }
// block must reference the same signature SIG as the surrounding
// call. Cross-signature alternatives in the response block are a
// static error - the response can never come from the called
// signature.
func callResponseSignatureDiags(call *syntax.CallExpr, body *syntax.BlockStmt) []Diagnostic {
	if call == nil || body == nil {
		return nil
	}
	callerSig := firstArgSignatureName(call)
	if callerSig == "" {
		return nil
	}
	var diags []Diagnostic
	for _, st := range body.Stmts {
		cc, ok := st.(*syntax.CommClause)
		if !ok || cc == nil || cc.Comm == nil {
			continue
		}
		es, ok := cc.Comm.(*syntax.ExprStmt)
		if !ok || es.Expr == nil {
			continue
		}
		inner, ok := es.Expr.(*syntax.CallExpr)
		if !ok {
			continue
		}
		op := selectorOpName(inner.Fun)
		if op != "getreply" && op != "catch" {
			continue
		}
		gotSig := responseSignatureName(inner, op)
		if gotSig == "" || gotSig == callerSig {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "call-response-signature-mismatch",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s alternative references signature %q but the enclosing call uses %q (ETSI 22.3.1 h)",
				op, gotSig, callerSig),
			Node: inner,
			Span: syntax.SpanOf(inner),
		})
	}
	return diags
}

// firstArgSignatureName extracts the signature identifier from the
// first argument of `p.call(SIG:...)`. Returns "" if the call has
// no args or the first arg is not the `SIG:<template>` shape.
func firstArgSignatureName(call *syntax.CallExpr) string {
	if call == nil || call.Args == nil || len(call.Args.List) == 0 {
		return ""
	}
	return callSignatureName(call.Args.List[0])
}

// responseSignatureName extracts the signature identifier from a
// `p.getreply(SIG:...)` (first arg, BinaryExpr) or `p.catch(SIG,
// ...)` (first arg, plain Ident). Returns "" if not in the
// expected shape.  The `catch (timeout)` special form is treated
// as "no signature attached" - "timeout" is a keyword that catches
// the enclosing call's timer expiration (ETSI 22.3.3), not a
// signature reference.
func responseSignatureName(call *syntax.CallExpr, op string) string {
	if call == nil || call.Args == nil || len(call.Args.List) == 0 {
		return ""
	}
	first := call.Args.List[0]
	switch op {
	case "getreply":
		return callSignatureName(first)
	case "catch":
		if id, ok := first.(*syntax.Ident); ok && id != nil && id.Tok != nil {
			name := id.String()
			if name == "timeout" {
				return ""
			}
			return name
		}
	}
	return ""
}

// selectorOpName returns the trailing selector identifier of a
// `port.<op>` expression, or "" otherwise.
func selectorOpName(e syntax.Expr) string {
	sel, ok := e.(*syntax.SelectorExpr)
	if !ok || sel == nil || sel.Sel == nil {
		return ""
	}
	id, ok := sel.Sel.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return ""
	}
	return id.String()
}

// exprStmtCallExpr returns the inner CallExpr if the given
// ExprStmt is a port.call(...) expression. nil otherwise.
func exprStmtCallExpr(es *syntax.ExprStmt) *syntax.CallExpr {
	if es == nil || es.Expr == nil {
		return nil
	}
	ce, ok := es.Expr.(*syntax.CallExpr)
	if !ok {
		return nil
	}
	sel, ok := ce.Fun.(*syntax.SelectorExpr)
	if !ok || sel.Sel == nil {
		return nil
	}
	id, ok := sel.Sel.(*syntax.Ident)
	if !ok || id.Tok == nil || id.String() != "call" {
		return nil
	}
	return ce
}

func callTimerValueDiags(call *syntax.CallExpr, body *syntax.BlockStmt) []Diagnostic {
	if call == nil || call.Args == nil {
		return nil
	}
	args := call.Args.List
	if len(args) < 2 {
		return nil
	}
	timerArg := args[1]
	if timerArg == nil {
		return nil
	}
	if isNowaitArg(timerArg) {
		if body != nil {
			return []Diagnostic{{
				Code:     "call-nowait-with-response-block",
				Severity: SeverityError,
				Message: "call(..., nowait) is non-blocking and must " +
					"not have a response-handling block (ETSI 22.3.1)",
				Node: body,
				Span: syntax.SpanOf(body),
			}}
		}
		return nil
	}
	if !isFloatishCallTimer(timerArg) {
		return []Diagnostic{{
			Code:     "call-timer-value-not-float",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"CallTimerValue %q must be of type float (ETSI 22.3.1 g)",
				timerArg.LastTok().String()),
			Node: timerArg,
			Span: syntax.SpanOf(timerArg),
		}}
	}
	return nil
}

func isNowaitArg(e syntax.Expr) bool {
	id, ok := e.(*syntax.Ident)
	if !ok || id.Tok == nil {
		return false
	}
	return id.String() == "nowait"
}

// isFloatishCallTimer accepts anything we can't statically prove
// is non-float. Concretely: floats, idents (could be a `const float`),
// selector/index/call expressions (could be a function returning float).
// We only reject obvious shape mismatches: integer / bitstring /
// boolean / character-string literals and composite literals.
func isFloatishCallTimer(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil {
			return true
		}
		switch x.Tok.Kind() {
		case syntax.FLOAT:
			return true
		case syntax.INT, syntax.BSTRING, syntax.STRING,
			syntax.TRUE, syntax.FALSE:
			return false
		default:
			return true
		}
	case *syntax.CompositeLiteral:
		return false
	}
	return true
}
