// call_context_rules.go follows the functions a control part, a @control
// function or a component type declaration invokes, directly or
// indirectly, and reports the operations not allowed there (ETSI ES
// 201 873-1):
//
//   - configuration operations (create, connect, disconnect, map, unmap)
//     are used only in test cases and behaviour they invoke or a PTC
//     runs: not in control behaviour, nor in declarations inside a
//     component type definition or the functions those invoke (21.0 a,
//     b);
//   - setverdict is not used in control behaviour (16.1.5 e, table 15).
//
// A control part's own statements are control_part_ops.go's. Behaviour a
// call only names, the argument of execute, activate or a component's
// start, is not invoked by the caller and is not followed.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkCallContextRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	funcs := map[string]*syntax.FuncDecl{}
	comps := map[string]bool{}
	// External functions have no body to follow.
	for _, d := range mod.Defs {
		if ct, ok := d.Def.(*syntax.ComponentTypeDecl); ok && ct.Name != nil {
			comps[ct.Name.String()] = true
		}
		if fn, ok := d.Def.(*syntax.FuncDecl); ok && fn.Name != nil && fn.Body != nil && fn.KindTok != nil && fn.KindTok.Kind() == syntax.FUNCTION {
			funcs[fn.Name.String()] = fn
		}
	}
	// reach is every function of funcs root invokes, directly or
	// indirectly.
	reach := func(roots ...syntax.Node) map[*syntax.FuncDecl]bool {
		seen := map[*syntax.FuncDecl]bool{}
		var visit func(n syntax.Node)
		visit = func(n syntax.Node) {
			for _, callee := range invokedFuncs(n, funcs) {
				if !seen[callee] {
					seen[callee] = true
					visit(callee.Body)
				}
			}
		}
		for _, r := range roots {
			visit(r)
		}
		return seen
	}

	var diags []Diagnostic
	report := func(body syntax.Node, setverdict bool, where, clause string) {
		syntax.Inspect(body, func(n syntax.Node) bool {
			if op := configurationOp(n, comps); op != "" {
				diags = append(diags, Diagnostic{
					Code:     "configuration-operation-context",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s: a configuration operation is not used in %s (ETSI %s)", op, where, clause),
					Node:     n,
					Span:     syntax.SpanOf(n),
				})
			}
			if call, ok := n.(*syntax.CallExpr); ok && setverdict && syntax.Name(call.Fun) == "setverdict" {
				diags = append(diags, Diagnostic{
					Code:     "setverdict-in-control",
					Severity: SeverityError,
					Message:  fmt.Sprintf("setverdict is not used in %s (ETSI 16.1.5, table 15)", where),
					Node:     n,
					Span:     syntax.SpanOf(n),
				})
			}
			return !isBehaviourArgument(n)
		})
	}

	// Control behaviour: control parts and @control functions.
	var controlRoots []syntax.Node
	controlFuncs := map[*syntax.FuncDecl]bool{}
	for _, d := range mod.Defs {
		switch x := d.Def.(type) {
		case *syntax.ControlPart:
			if x.Body != nil {
				controlRoots = append(controlRoots, x.Body)
			}
		case *syntax.FuncDecl:
			if x.Body != nil && x.Modif != nil && x.Modif.String() == "@control" {
				controlRoots = append(controlRoots, x.Body)
				controlFuncs[x] = true
			}
		}
	}
	fromControl := reach(controlRoots...)
	for fn := range controlFuncs {
		fromControl[fn] = true
	}
	for fn := range fromControl {
		report(fn.Body, true, fmt.Sprintf("control behaviour (%s)", fn.Name), "21.0 a")
	}

	// Declarations inside component type definitions.
	var compRoots []syntax.Node
	for _, d := range mod.Defs {
		if ct, ok := d.Def.(*syntax.ComponentTypeDecl); ok && ct.Body != nil {
			compRoots = append(compRoots, ct.Body)
			report(ct.Body, false, fmt.Sprintf("a declaration of component type %s", ct.Name), "21.0 b")
		}
	}
	for fn := range reach(compRoots...) {
		if !fromControl[fn] {
			report(fn.Body, false, fmt.Sprintf("%s, invoked from a component type declaration", fn.Name), "21.0 b")
		}
	}
	return dedupeDiagnostics(diags)
}

// invokedFuncs is the functions of funcs n calls, not through the
// behaviour argument of execute, activate or start.
func invokedFuncs(n syntax.Node, funcs map[string]*syntax.FuncDecl) []*syntax.FuncDecl {
	var out []*syntax.FuncDecl
	if n == nil {
		return out
	}
	syntax.Inspect(n, func(n syntax.Node) bool {
		if isBehaviourArgument(n) {
			return false
		}
		if call, ok := n.(*syntax.CallExpr); ok {
			if id, ok := call.Fun.(*syntax.Ident); ok {
				if fn := funcs[id.String()]; fn != nil {
					out = append(out, fn)
				}
			}
		}
		return true
	})
	return out
}

// isBehaviourArgument reports whether n is a call whose argument is a
// behaviour it names rather than invokes: execute(tc()), activate(a()),
// c.start(f()).
func isBehaviourArgument(n syntax.Node) bool {
	call, ok := n.(*syntax.CallExpr)
	if !ok {
		return false
	}
	switch f := call.Fun.(type) {
	case *syntax.Ident:
		name := f.String()
		return name == "execute" || name == "activate"
	case *syntax.SelectorExpr:
		return syntax.Name(f.Sel) == "start" && call.Args != nil && len(call.Args.List) == 1 && isCall(call.Args.List[0])
	}
	return false
}

func isCall(e syntax.Expr) bool {
	_, ok := e.(*syntax.CallExpr)
	return ok
}

// configurationOp names the configuration operation n is: T.create for a
// component type T of comps, or connect, disconnect, map or unmap.
func configurationOp(n syntax.Node, comps map[string]bool) string {
	switch x := n.(type) {
	case *syntax.SelectorExpr:
		if syntax.Name(x.Sel) == "create" && comps[syntax.Name(x.X)] {
			return "create"
		}
	case *syntax.CallExpr:
		switch name := syntax.Name(x.Fun); name {
		case "connect", "disconnect", "map", "unmap":
			if _, ok := x.Fun.(*syntax.Ident); ok {
				return name
			}
		}
	}
	return ""
}
