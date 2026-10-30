// function_spec_rules.go enforces a handful of static checks on
// the optional `runs on`, `mtc`, and `system` clauses that can
// accompany a function / altstep / testcase declaration (ETSI ES
// 201 873-1 clause 16.1).
//
// Narrow rules are implemented:
//
//   - a function called, directly or through others, from the
//     control part or a control function shall have no `mtc` or
//     `system` clause (restriction c; NegSem_1601_toplevel_007 /
//     _008). Declaring one is allowed.
//
//   - A function or altstep without a `runs on` clause shall not
//     invoke a function or altstep that *does* carry one
//     (NegSem_1601_toplevel_003 and the symmetric _006 / _009).
//     We catch the most common case: the calling function has no
//     runs-on AND it invokes a sibling module-level function whose
//     declaration we can resolve.
//
// The rule is conservative: imports we can't resolve and dynamic
// dispatches through references silently pass.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkFunctionSpecRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	if mod == nil {
		return diags
	}

	// Pre-pass: cache the runs-on / mtc / system status of every
	// module-level function/altstep/testcase.
	type fnInfo struct {
		runsOn   string
		hasMtc   bool
		hasSys   bool
		isTC     bool
		external bool
	}
	infos := map[string]fnInfo{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}
		info := fnInfo{external: fn.External != nil && fn.External.Kind() == syntax.EXTERNAL}
		if fn.KindTok != nil && fn.KindTok.Kind() == syntax.TESTCASE {
			info.isTC = true
		}
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			info.runsOn = syntax.Name(fn.RunsOn.Comp)
		}
		if fn.Mtc != nil {
			info.hasMtc = true
		}
		if fn.System != nil {
			info.hasSys = true
		}
		infos[fn.Name.String()] = info
	}

	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		info := infos[syntax.Name(fn.Name)]
		// Rule 2: a runs-on-less function calling a runs-on
		// function. Skip externals (no body) and testcases.
		if info.isTC || info.external || info.runsOn != "" || fn.Body == nil {
			continue
		}
		started := startedBehaviourCalls(fn.Body)
		activated := activatedBehaviourCalls(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			if started[ce] || activated[ce] {
				return true
			}
			id, ok := ce.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			calleeInfo, ok := infos[id.String()]
			if !ok {
				return true
			}
			// Testcase invocations are always wrapped in
			// `execute(...)` so they do not propagate the
			// runs-on requirement to the caller. We skip
			// them to avoid flagging Sem_050402's `f_caller`
			// pattern.
			if calleeInfo.runsOn == "" || calleeInfo.isTC {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "function-no-runs-on-invokes-runs-on",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q has no `runs on` clause but invokes %q which runs on %s (ETSI 16.1 a)",
					syntax.Name(fn.Name), id.String(), calleeInfo.runsOn),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}

	// Rule 1: a function called, directly or through others, from
	// the control part or a control function has no mtc or system
	// clause (ETSI 16.1 restriction c). A function may otherwise
	// have both.
	diags = append(diags, controlCallsWithMtcOrSystem(mod)...)

	// Rule 2b: in a runs-on-less function / altstep, any
	// port operation whose receiver is a bare identifier we
	// can't bind locally must refer to an implicit component
	// member - which requires a runs-on. Catches
	// NegSem_1602_toplevel_005 and friends.
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		info := infos[syntax.Name(fn.Name)]
		if info.isTC || info.external || info.runsOn != "" || fn.Body == nil {
			continue
		}
		paramTypes := collectFormalParamTypeNames(fn.Params)
		varTypes := collectVarDeclaredTypeNames(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			sel, ok := n.(*syntax.SelectorExpr)
			if !ok {
				return true
			}
			opIdent, ok := sel.Sel.(*syntax.Ident)
			if !ok || !portOpsBuiltin[opIdent.String()] {
				return true
			}
			recv, ok := sel.X.(*syntax.Ident)
			if !ok {
				return true
			}
			name := recv.String()
			switch name {
			case "mtc", "self", "system":
				// These three are obviously
				// component-reference qualifiers, but
				// they're only legal inside something
				// that does have a runs-on - the
				// caller's existing check flags them
				// for us, so we simply pass through.
				return true
			}
			// We deliberately do NOT short-circuit on
			// `any port` / `all port` / `any component` /
			// `all component` / `any timer` / `all timer`:
			// those qualifiers presuppose a runs-on for
			// the enclosing behaviour (ETSI 22.5 / 22.6 /
			// 23.6), and the conformance suite expects an
			// error when they appear inside a runs-on-less
			// altstep (see NegSem_1602_toplevel_007).
			if _, ok := varTypes[name]; ok {
				return true
			}
			if _, ok := paramTypes[name]; ok {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "port-op-no-runs-on-on-bare-name",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q has no `runs on` clause but applies %q to %q (implicit component member, ETSI 16.1 / 22)",
					syntax.Name(fn.Name), opIdent.String(), name),
				Node: sel,
				Span: syntax.SpanOf(sel),
			})
			return true
		})
	}

	// Rule 3: control parts cannot invoke runs-on functions
	// (NegSem_1601_toplevel_004 / _009).
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		ctrl, ok := d.Def.(*syntax.ControlPart)
		if !ok || ctrl.Body == nil {
			continue
		}
		syntax.Inspect(ctrl.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			id, ok := ce.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			calleeInfo, ok := infos[id.String()]
			if !ok {
				return true
			}
			if calleeInfo.runsOn == "" || calleeInfo.isTC {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "control-invokes-runs-on",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"control part invokes %q which runs on %s (ETSI 16.1 a / 26.1)",
					id.String(), calleeInfo.runsOn),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}

	return diags
}

// startedBehaviourCalls returns the set of CallExprs that appear
// as the function-instance argument of a `<comp>.start(<call>)`
// or `<comp>.call(<sig>:<tmpl>, <call>)` invocation. Behaviours
// passed to start/call run on the target component, so the spec
// (ETSI 16.1 restriction f) explicitly allows that path even from
// a runs-on-less caller.
func startedBehaviourCalls(body *syntax.BlockStmt) map[*syntax.CallExpr]bool {
	out := map[*syntax.CallExpr]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		if opIdent.String() != "start" || ce.Args == nil {
			return true
		}
		for _, arg := range ce.Args.List {
			if inner, ok := arg.(*syntax.CallExpr); ok {
				out[inner] = true
			}
		}
		return true
	})
	return out
}

// activatedBehaviourCalls returns the set of CallExprs that
// appear as the argument of a top-level `activate(<altstep>)`
// invocation, mirroring the start-based exemption above
// (ETSI 16.1 restriction f covers activated altsteps too).
func activatedBehaviourCalls(body *syntax.BlockStmt) map[*syntax.CallExpr]bool {
	out := map[*syntax.CallExpr]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id.String() != "activate" || ce.Args == nil {
			return true
		}
		for _, arg := range ce.Args.List {
			if inner, ok := arg.(*syntax.CallExpr); ok {
				out[inner] = true
			}
		}
		return true
	})
	return out
}

// controlCallsWithMtcOrSystem reports the calls of the control part and
// of control functions that reach, directly or through other functions,
// one with an mtc or system clause (ETSI 16.1 restriction c). Testcases,
// which the control part executes, are not followed.
func controlCallsWithMtcOrSystem(mod *syntax.Module) []Diagnostic {
	self := syntax.Name(mod.Name)
	funcs := map[string]*syntax.FuncDecl{}
	var roots []*syntax.BlockStmt
	var defs func(list []*syntax.ModuleDef)
	defs = func(list []*syntax.ModuleDef) {
		for _, d := range list {
			if d == nil {
				continue
			}
			switch x := d.Def.(type) {
			case *syntax.GroupDecl:
				defs(x.Defs)
			case *syntax.ControlPart:
				if x.Body != nil {
					roots = append(roots, x.Body)
				}
			case *syntax.FuncDecl:
				if x.Name != nil {
					funcs[x.Name.String()] = x
				}
				if x.IsControl() && x.Body != nil {
					roots = append(roots, x.Body)
				}
			}
		}
	}
	defs(mod.Defs)

	// callee names the function of this module a call calls, or "".
	callee := func(ce *syntax.CallExpr) string {
		switch f := ce.Fun.(type) {
		case *syntax.Ident:
			return f.String()
		case *syntax.SelectorExpr:
			if syntax.Name(f.X) == self {
				return syntax.Name(f.Sel)
			}
		}
		return ""
	}
	// reaches is the first function with an mtc or system clause a
	// call of name reaches, name itself included, or "".
	reaches := func(name string) string {
		seen := map[string]bool{}
		queue := []string{name}
		for len(queue) > 0 {
			n := queue[0]
			queue = queue[1:]
			fd := funcs[n]
			if seen[n] || fd == nil || fd.KindTok == nil || fd.KindTok.Kind() == syntax.TESTCASE {
				continue
			}
			seen[n] = true
			if fd.Mtc != nil || fd.System != nil {
				return n
			}
			if fd.Body != nil {
				syntax.Inspect(fd.Body, func(m syntax.Node) bool {
					if ce, ok := m.(*syntax.CallExpr); ok {
						if c := callee(ce); c != "" {
							queue = append(queue, c)
						}
					}
					return true
				})
			}
		}
		return ""
	}
	var diags []Diagnostic
	for _, root := range roots {
		syntax.Inspect(root, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			name := callee(ce)
			r := ""
			if name != "" {
				r = reaches(name)
			}
			if r == "" {
				return true
			}
			via := ""
			if r != name {
				via = fmt.Sprintf(", through %q", name)
			}
			diags = append(diags, Diagnostic{
				Code:     "control-calls-function-with-mtc-or-system",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"the control part calls %q%s, which has an mtc or system clause (ETSI 16.1 restriction c)", r, via),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}
	return diags
}
