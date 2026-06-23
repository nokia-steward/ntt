// activate_rules.go enforces ETSI ES 201 873-1 clause 20.5.2
// "The activate operation":
//
//   - The argument of `activate(...)` shall refer to an altstep.
//     Activating a regular function (even one whose body is an
//     `alt` block) is rejected.
//
//   - Altsteps used as activate-defaults shall only have `in`,
//     port, or timer parameters. `out` or `inout` formal
//     parameters are forbidden.
//
// The check is module-local: when the activate-argument name
// cannot be resolved to a same-module FuncDecl we conservatively
// skip the case, leaving cross-module rejection to whatever
// future name-resolution pass eventually lands.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkActivateRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	funcs := collectFunctionSignatures(mod)
	if len(funcs) == 0 {
		return nil
	}
	extends := collectComponentExtensionGraph(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		callerComp := runsOnComponent(fn)
		diags = append(diags, checkActivateInBody(fn.Body, funcs, callerComp, extends)...)
	}
	// Also walk the module's control part body, if any.
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ctrl, ok := d.Def.(*syntax.ControlPart)
		if !ok || ctrl.Body == nil {
			continue
		}
		diags = append(diags, checkActivateInBody(ctrl.Body, funcs, "", extends)...)
	}
	return diags
}

// runsOnComponent returns the component-type name from a function /
// testcase / altstep `runs on T` clause, or "" when the clause is
// absent (e.g. module-control invocation).
func runsOnComponent(fn *syntax.FuncDecl) string {
	if fn == nil || fn.RunsOn == nil || fn.RunsOn.Comp == nil {
		return ""
	}
	return identName(fn.RunsOn.Comp)
}

// collectComponentExtensionGraph returns a `child -> {parents}` map
// across `type component C extends A, B { ... }` declarations.
// Parent components are stored by literal type-name only; the
// returned map does NOT transitively close - callers should walk
// ancestors via a separate traversal helper.
func collectComponentExtensionGraph(mod *syntax.Module) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ctd, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ctd == nil || ctd.Name == nil {
			continue
		}
		ps := map[string]bool{}
		for _, p := range ctd.Extends {
			if n := identName(p); n != "" {
				ps[n] = true
			}
		}
		out[ctd.Name.String()] = ps
	}
	return out
}

// componentCompatible reports whether activating an altstep that
// runs on `target` is legal from a caller that runs on `caller`.
// The relation is reflexive and transitive across `extends` edges
// in either direction: a parent component may invoke / activate a
// child's altstep iff the runtime instance is the child (here we
// allow both directions as a static approximation, matching the
// suite's positive tests).
func componentCompatible(caller, target string, extends map[string]map[string]bool) bool {
	if caller == "" || target == "" || caller == target {
		return true
	}
	if hasComponentAncestor(caller, target, extends, map[string]bool{}) {
		return true
	}
	return hasComponentAncestor(target, caller, extends, map[string]bool{})
}

func hasComponentAncestor(name, ancestor string, extends map[string]map[string]bool, seen map[string]bool) bool {
	if seen[name] {
		return false
	}
	seen[name] = true
	for parent := range extends[name] {
		if parent == ancestor {
			return true
		}
		if hasComponentAncestor(parent, ancestor, extends, seen) {
			return true
		}
	}
	return false
}

func checkActivateInBody(body *syntax.BlockStmt, funcs map[string]*syntax.FuncDecl, callerComp string, extends map[string]map[string]bool) []Diagnostic {
	var diags []Diagnostic
	seen := map[syntax.Node]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		name, ok := ce.Fun.(*syntax.Ident)
		if !ok || name.String() != "activate" {
			return true
		}
		if ce.Args == nil || len(ce.Args.List) != 1 {
			return true
		}
		inner, ok := ce.Args.List[0].(*syntax.CallExpr)
		if !ok || inner.Fun == nil {
			return true
		}
		var targetName string
		switch t := inner.Fun.(type) {
		case *syntax.Ident:
			targetName = t.String()
		default:
			return true
		}
		decl, found := funcs[targetName]
		if !found || decl == nil || decl.KindTok == nil {
			return true
		}
		if seen[inner] {
			return true
		}
		seen[inner] = true
		switch decl.KindTok.Kind() {
		case syntax.ALTSTEP:
			// must not have out / inout params
			if decl.Params != nil {
				for _, p := range decl.Params.List {
					if p == nil || p.Direction == nil {
						continue
					}
					k := p.Direction.Kind()
					if k == syntax.OUT || k == syntax.INOUT {
						diags = append(diags, Diagnostic{
							Code:     "activate-altstep-out-param",
							Severity: SeverityError,
							Message:  "altstep activated as default must not have `out` or `inout` parameters (ETSI 20.5.2)",
							Node:     inner,
							Span:     syntax.SpanOf(inner),
						})
						break
					}
				}
			}
			// runs-on compatibility: altstep's component must be
			// reachable from the caller's component via the
			// `extends` graph (either direction).
			targetComp := runsOnComponent(decl)
			if callerComp != "" && targetComp != "" &&
				!componentCompatible(callerComp, targetComp, extends) {
				diags = append(diags, Diagnostic{
					Code:     "activate-runs-on-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`activate` target altstep runs on %q but the caller runs on %q; the component types are unrelated (ETSI 20.5.2)",
						targetComp, callerComp),
					Node: inner,
					Span: syntax.SpanOf(inner),
				})
			}
		case syntax.FUNCTION, syntax.TESTCASE:
			diags = append(diags, Diagnostic{
				Code:     "activate-non-altstep",
				Severity: SeverityError,
				Message:  "argument of `activate` must reference an altstep (ETSI 20.5.2)",
				Node:     inner,
				Span:     syntax.SpanOf(inner),
			})
		}
		return true
	})
	return diags
}
