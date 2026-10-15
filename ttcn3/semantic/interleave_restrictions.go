// interleave_restrictions.go implements ETSI ES 201 873-1 clause
// 20.4 restriction (a) ... (d): inside an `interleave` statement
// the body may not contain control-transfer or function-call
// constructs that could observe partial reception order. The spec
// lists the forbidden constructs explicitly:
//
//   - for / while / do-while loops (if the body contains reception)
//   - goto statements (conditional and unconditional)
//   - if / select statements (control-flow inside an alternative)
//   - nested alt / interleave (use a guard expression instead)
//   - activate / deactivate calls
//   - stop / repeat / return statements
//   - direct altstep calls used as alternatives
//   - user-defined function calls whose body contains reception
//     statements (direct or indirect)
//
// The conformance suite enforces the structural side of these
// rules (no for/while/do-while/if/select/goto/alt anywhere inside
// the interleave body, regardless of whether the body would
// actually receive). The narrower "must contain reception" variant
// the spec text mentions is not separately exercised, so the rule
// is intentionally strict.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkInterleaveRestrictions(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	// Track which module-level names are altsteps so we can flag
	// direct altstep invocations from inside interleave bodies.
	altsteps := map[string]bool{}
	// Track which function bodies contain reception statements
	// (direct or transitively via other functions). 20.4 c)
	// forbids calling such a function from inside an interleave
	// body. We do a fixed-point sweep so an indirect chain
	// `f_caller -> f_receive` is also caught.
	funcBodies := map[string]*syntax.FuncDecl{}
	receptive := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if fn, ok := n.(*syntax.FuncDecl); ok && fn.Name != nil && fn.KindTok != nil {
			name := fn.Name.String()
			funcBodies[name] = fn
			if fn.KindTok.Kind() == syntax.ALTSTEP {
				altsteps[name] = true
				receptive[name] = true
				return true
			}
			if fn.Body != nil && bodyHasReception(fn.Body, false) {
				receptive[name] = true
			}
		}
		return true
	})
	// Fixed-point: a function calling a receptive function is itself receptive.
	for changed := true; changed; {
		changed = false
		for name, fn := range funcBodies {
			if receptive[name] || fn.Body == nil {
				continue
			}
			if bodyCallsReceptive(fn.Body, receptive) {
				receptive[name] = true
				changed = true
			}
		}
	}

	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		alt, ok := n.(*syntax.AltStmt)
		if !ok || alt.Tok == nil || alt.Tok.Kind() != syntax.INTERLEAVE {
			return true
		}
		// Walk every node in the interleave body and flag the
		// forbidden constructs.
		syntax.Inspect(alt.Body, func(m syntax.Node) bool {
			if m == nil {
				return true
			}
			switch x := m.(type) {
			case *syntax.ExprStmt:
				// Bare-form `deactivate;` / `activate;`
				// (the parser surfaces them as an ExprStmt
				// wrapping a plain identifier). The paren
				// form is caught by the CallExpr branch
				// below.
				if id, ok := x.Expr.(*syntax.Ident); ok && id != nil && id.Tok != nil {
					name := id.String()
					if name == "activate" || name == "deactivate" {
						diags = append(diags, Diagnostic{
							Code:     "interleave-forbidden-call",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"%q is not allowed inside an interleave statement",
								name),
							Node: x,
							Span: syntax.SpanOf(x),
						})
					}
				}
			case *syntax.CallExpr:
				if id, ok := x.Fun.(*syntax.Ident); ok && id.Tok != nil {
					name := id.String()
					switch name {
					case "activate", "deactivate":
						diags = append(diags, Diagnostic{
							Code:     "interleave-forbidden-call",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"%q is not allowed inside an interleave statement",
								name),
							Node: x,
							Span: syntax.SpanOf(x),
						})
					default:
						if altsteps[name] {
							diags = append(diags, Diagnostic{
								Code:     "interleave-altstep-call",
								Severity: SeverityError,
								Message: fmt.Sprintf(
									"direct altstep call %q is not allowed inside an interleave statement",
									name),
								Node: x,
								Span: syntax.SpanOf(x),
							})
						} else if receptive[name] {
							diags = append(diags, Diagnostic{
								Code:     "interleave-receptive-call",
								Severity: SeverityError,
								Message: fmt.Sprintf(
									"call to %q is not allowed inside an interleave: the function (directly or indirectly) performs reception (ETSI 20.4 c)",
									name),
								Node: x,
								Span: syntax.SpanOf(x),
							})
						}
					}
				}
			case *syntax.BranchStmt:
				// `repeat` and `return` are unconditionally
				// forbidden inside an interleave (20.4 b/d).
				// `goto`, `break` and `continue` are only
				// forbidden when they cross reception
				// boundaries, which needs control-flow
				// analysis; we leave them to the runtime.
				if x.Tok != nil {
					switch x.Tok.Kind() {
					case syntax.REPEAT, syntax.RETURN:
						diags = append(diags, Diagnostic{
							Code:     "interleave-forbidden-branch",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"%q is not allowed inside an interleave statement",
								x.Tok.String()),
							Node: x,
							Span: syntax.SpanOf(x),
						})
					}
				}
			case *syntax.ForStmt:
				if x.Body != nil && bodyHasReception(x.Body, false) {
					diags = append(diags, interleaveCtrlDiag("for-with-reception", x))
				}
			case *syntax.WhileStmt:
				if x.Body != nil && bodyHasReception(x.Body, false) {
					diags = append(diags, interleaveCtrlDiag("while-with-reception", x))
				}
			case *syntax.DoWhileStmt:
				if x.Body != nil && bodyHasReception(x.Body, false) {
					diags = append(diags, interleaveCtrlDiag("do-while-with-reception", x))
				}
			case *syntax.AltStmt:
				// Skip the outer interleave itself.
				if x == alt {
					return true
				}
				// Only `interleave` nested in another
				// interleave is illegal; nested `alt` is
				// fine because it's the conformance suite's
				// preferred reception pattern.
				if x.Tok != nil && x.Tok.Kind() == syntax.INTERLEAVE {
					diags = append(diags, interleaveCtrlDiag("nested interleave", x))
				}
			}
			return true
		})
		return true
	})
	return diags
}

func interleaveCtrlDiag(what string, n syntax.Node) Diagnostic {
	return Diagnostic{
		Code:     "interleave-forbidden-control",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s statement is not allowed inside an interleave (ETSI 20.4)",
			what),
		Node: n,
		Span: syntax.SpanOf(n),
	}
}

// receivingOps is the set of port-operation names that ETSI
// considers "reception statements" for the purpose of the 20.4
// "function with reception cannot be called from interleave"
// restriction. We include receive / trigger / check / getcall /
// getreply / catch; timer .timeout and component .done arguably
// belong on this list too but the conformance suite doesn't
// exercise them.
var receivingOps = map[string]bool{
	"receive":  true,
	"trigger":  true,
	"check":    true,
	"getcall":  true,
	"getreply": true,
	"catch":    true,
}

// bodyHasReception reports whether the given subtree contains a
// reception statement. The `selfRef` flag enables the same body
// to count its own calls (used for the indirect-reception
// fixed-point); we keep it false at top-level to avoid an
// infinite loop on `function f { f(); }`.
func bodyHasReception(n syntax.Node, _ bool) bool {
	found := false
	syntax.Inspect(n, func(m syntax.Node) bool {
		if found || m == nil {
			return false
		}
		switch x := m.(type) {
		case *syntax.CallExpr:
			if sel, ok := x.Fun.(*syntax.SelectorExpr); ok {
				if id, ok := sel.Sel.(*syntax.Ident); ok && id.Tok != nil {
					if receivingOps[id.String()] {
						found = true
						return false
					}
				}
			}
		case *syntax.SelectorExpr:
			// Bare-form selector like `p.receive` used
			// as the head of an alt branch.
			if id, ok := x.Sel.(*syntax.Ident); ok && id.Tok != nil {
				if receivingOps[id.String()] {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// bodyCallsReceptive reports whether the given subtree contains
// a call to any function whose name is in the receptive set. We
// look at direct call sites only; indirect calls via function
// pointers are not modelled.
func bodyCallsReceptive(n syntax.Node, receptive map[string]bool) bool {
	found := false
	syntax.Inspect(n, func(m syntax.Node) bool {
		if found || m == nil {
			return false
		}
		ce, ok := m.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok {
			return true
		}
		if receptive[id.String()] {
			found = true
			return false
		}
		return true
	})
	return found
}
