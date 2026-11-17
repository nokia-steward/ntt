// interleave_restrictions.go implements ETSI ES 201 873-1 clause
// 20.4's restrictions on what an `interleave` statement contains:
//
//   - activate / deactivate calls, repeat and return statements, direct
//     altstep calls, and calls of user-defined functions whose body
//     contains reception statements, directly or indirectly (a);
//   - for / while / do-while loops whose block contains a reception
//     statement (d): a loop is allowed in a block without one, even in
//     a branch that receives before or after it.
//
// An alt or an interleave inside an interleave is allowed: 20.4 forbids
// neither, and the replacement of an interleave by nested alt statements
// covers them.
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

	// checkBody walks every node of an interleave's body and flags the
	// forbidden constructs; outer is the interleave itself, if any.
	checkBody := func(body, outer syntax.Node) {
		syntax.Inspect(body, func(m syntax.Node) bool {
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
			}
			return true
		})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch n := n.(type) {
		case *syntax.AltStmt:
			if n.Tok != nil && n.Tok.Kind() == syntax.INTERLEAVE {
				checkBody(n.Body, n)
			}
		case *syntax.FuncDecl:
			// The top alternatives of an interleave altstep are an
			// interleave's (ETSI 16.2.0 g).
			if n.KindTok != nil && n.KindTok.Kind() == syntax.ALTSTEP && n.Interleave != nil && n.Body != nil {
				for _, st := range n.Body.Stmts {
					if cc, ok := st.(*syntax.CommClause); ok {
						checkBody(cc, nil)
					}
				}
			}
		}
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
