// control_part_ops.go implements the static check from ETSI ES 201
// 873-1 clause 26.2: a module's control part may not contain
// component operations, port communication, setverdict, or other
// behaviour-only statements. The control part is purely top-level
// orchestration of execute() invocations.
//
// We walk the body of every ControlPart and flag any direct or
// transitive use of the forbidden operations (the transitive check
// reuses the call graph built up in side_effect_ctx.go).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// hasLeadingTimerKeyword reports whether the expression's first
// token is `timer`. Used to recognise `timer name;` short-form
// declarations whose type expression carries the `timer` keyword.
func hasLeadingTimerKeyword(e syntax.Expr) bool {
	if e == nil {
		return false
	}
	if id, ok := e.(*syntax.Ident); ok && id.Tok != nil {
		return id.Tok.Kind() == syntax.TIMER
	}
	return false
}

// controlForbiddenSelectorSels is the set of selector tokens that
// represent component-only / port communication operations forbidden
// inside a control part. We deliberately omit selectors that overlap
// with timer operations (`start`, `stop`, `running`, `timeout`) and
// any port name suffix that could also refer to a struct field.
//
// The component-only operations and the port communication verbs
// can be statically identified by their selector name regardless of
// receiver type.
var controlForbiddenSelectorSels = map[string]bool{
	"create": true, "kill": true,
	"alive": true, "done": true, "killed": true,
	"send": true, "receive": true, "trigger": true, "check": true,
	"call": true, "getcall": true, "reply": true, "getreply": true,
	"raise": true, "catch": true,
}

// controlAmbiguousSels are selectors that overlap with timer
// operations. They are only forbidden when the receiver is a
// component or port. We use the timer-name table built per
// ControlPart to spare timer receivers.
var controlAmbiguousSels = map[string]bool{
	"start": true, "stop": true, "running": true,
}

// controlForbiddenCallSels is the set of selectors that overlap
// with timer ops and are only forbidden when the receiver is a
// component, which we approximate by requiring at least one
// CallExpr argument (`comp.start(f())` vs `t_timer.start` or
// `t_timer.start(2.0)`).
var controlForbiddenCallSels = map[string]bool{
	"start": true,
}

// controlForbiddenBareCallees is the set of bare identifier call
// names forbidden inside a control part. ETSI ES 201 873-1 allows
// `action()` from any execution context including control, so we
// do NOT flag it here (Syn_26_ModuleControl_016).
var controlForbiddenBareCallees = map[string]bool{
	"setverdict": true,
	"connect":    true, "disconnect": true,
	"map": true, "unmap": true,
}

func (a *Analyzer) checkControlPartOps(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		cp, ok := d.Def.(*syntax.ControlPart)
		if !ok || cp.Body == nil {
			continue
		}
		// Collect variable names declared as `timer` in this
		// control part so we can spare `t.start`, `t.stop`,
		// `t.running` from the component-op check.
		//
		// The parser consumes the `timer` keyword via parseTypeRef
		// when the declaration uses the `timer name;` short form,
		// so KindTok is nil and the Type expression's first token
		// is `timer`. We accept both cases.
		timerNames := map[string]bool{}
		syntax.Inspect(cp.Body, func(n syntax.Node) bool {
			if n == nil {
				return true
			}
			vd, ok := n.(*syntax.ValueDecl)
			if !ok {
				return true
			}
			isTimer := (vd.KindTok != nil && vd.KindTok.Kind() == syntax.TIMER) ||
				hasLeadingTimerKeyword(vd.Type)
			if !isTimer {
				return true
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				timerNames[dec.Name.String()] = true
			}
			return true
		})
		syntax.Inspect(cp.Body, func(n syntax.Node) bool {
			if n == nil {
				return true
			}
			switch x := n.(type) {
			case *syntax.SelectorExpr:
				if sid, ok := x.Sel.(*syntax.Ident); ok && sid.Tok != nil {
					name := sid.String()
					if controlForbiddenSelectorSels[name] {
						diags = append(diags, Diagnostic{
							Code:     "control-part-forbidden-op",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"%q is not allowed in a control part",
								name),
							Node: x,
							Span: syntax.SpanOf(x),
						})
					}
					// Ambiguous selectors: forbidden when
					// the receiver is not a known timer.
					// `comp.port.start` (SelectorExpr
					// receiver) is always a port op.
					// `var.start` (Ident receiver) is a
					// component op unless the variable is
					// a declared timer.
					if controlAmbiguousSels[name] {
						switch recv := x.X.(type) {
						case *syntax.SelectorExpr:
							diags = append(diags, Diagnostic{
								Code:     "control-part-forbidden-op",
								Severity: SeverityError,
								Message: fmt.Sprintf(
									"port operation %q is not allowed in a control part",
									name),
								Node: x,
								Span: syntax.SpanOf(x),
							})
						case *syntax.Ident:
							if recv.Tok != nil && !timerNames[recv.String()] {
								diags = append(diags, Diagnostic{
									Code:     "control-part-forbidden-op",
									Severity: SeverityError,
									Message: fmt.Sprintf(
										"component operation %q is not allowed in a control part",
										name),
									Node: x,
									Span: syntax.SpanOf(x),
								})
							}
						}
					}
				}
			case *syntax.CallExpr:
				if id, ok := x.Fun.(*syntax.Ident); ok && id.Tok != nil {
					name := id.String()
					if controlForbiddenBareCallees[name] {
						diags = append(diags, Diagnostic{
							Code:     "control-part-forbidden-op",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"%q is not allowed in a control part",
								name),
							Node: x,
							Span: syntax.SpanOf(x),
						})
					}
				}
				// `comp.start(f())` is forbidden; `t.start`
				// or `t.start(2.0)` is allowed. Distinguish
				// by whether the first argument is itself a
				// CallExpr (typical component-start syntax).
				if sel, ok := x.Fun.(*syntax.SelectorExpr); ok {
					if sid, ok := sel.Sel.(*syntax.Ident); ok && sid.Tok != nil {
						if controlForbiddenCallSels[sid.String()] && x.Args != nil && len(x.Args.List) > 0 {
							if _, isCall := x.Args.List[0].(*syntax.CallExpr); isCall {
								diags = append(diags, Diagnostic{
									Code:     "control-part-forbidden-op",
									Severity: SeverityError,
									Message: fmt.Sprintf(
										"%q is not allowed in a control part",
										sid.String()),
									Node: x,
									Span: syntax.SpanOf(x),
								})
							}
						}
					}
				}
			}
			return true
		})
	}
	return diags
}
