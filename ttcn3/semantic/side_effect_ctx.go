// side_effect_ctx.go implements the static check described in ETSI ES
// 201 873-1 clause 16.1.4: value-returning functions called from inside
// receiving communication operations, template values / template
// fields / in-line templates / actual parameters, or alt branch guards,
// must not perform any side-effecting operation - component / port /
// timer ops, action, setverdict, etc.
//
// The rule prevents a snapshot from changing under the matcher's feet
// and is the largest single bucket of negative-semantic tests in the
// ETSI conformance suite (~322 NegSem_160104_* files). We catch it at
// analysis time by:
//
//  1. Building a map of every module-level function to the set of
//     forbidden operations referenced in its body (transitively, so
//     g() calling h() inherits h's forbidden ops).
//  2. Walking each module for "restricted contexts" - TemplateDecl
//     value, alt branch guards, args to receive / trigger / check /
//     getreply / getcall / catch - and for every CallExpr in such a
//     context emitting a diagnostic if the callee has any forbidden
//     op.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// forbiddenSelectorSels lists the selector tokens that, when applied
// to a component / port / timer / verdict-issuing call, count as a
// side-effecting operation per clause 16.1.4. We do not type-check the
// receiver - the spec lists overlapping names (e.g. start can be a
// port / timer / component op) and any of those is an error in a
// restricted context.
var forbiddenSelectorSels = map[string]bool{
	// Component operations.
	"create": true, "running": true, "alive": true, "done": true,
	"killed": true,
	// Component / port / timer overlapping names.
	"start": true, "stop": true, "kill": true,
	// Port operations (state).
	"halt": true, "clear": true, "checkstate": true,
	// Port communication ops (16.1.4 lists them all).
	"send": true, "receive": true, "trigger": true,
	"call": true, "getcall": true, "reply": true, "getreply": true,
	"raise": true, "catch": true, "check": true,
	// Port configuration ops.
	"connect": true, "disconnect": true, "map": true, "unmap": true,
	// Timer state.
	"read": true, "timeout": true,
}

// forbiddenBareCallees lists keyword-shaped operations that surface
// as a plain identifier (not a selector). The TTCN-3 surface treats
// `connect(...)`, `map(...)`, `action(...)` etc. as bare-function-call
// syntax (no receiver), and clause 16.1.4 still lists them. `rnd` and
// related predefined functions are non-deterministic, so the spec
// also forbids them in restricted contexts (note 4).
var forbiddenBareCallees = map[string]bool{
	// Side-effect statements.
	"action": true, "setverdict": true, "stop": true,
	// Configuration ops invoked without a receiver.
	"connect": true, "disconnect": true, "map": true, "unmap": true,
	// Non-deterministic predefined helpers.
	"rnd": true,
	// Default-handling ops (rule i).
	"activate": true, "deactivate": true,
	// Rule (h): setencode at runtime mutates encoding state.
	"setencode": true,
}

// restrictedReceiveSels lists port-operation selectors whose call
// arguments are "receiving" positions per clause 16.1.4.
var restrictedReceiveSels = map[string]bool{
	"receive": true, "trigger": true, "check": true,
	"getreply": true, "getcall": true, "catch": true,
}

// collectComponentVars walks every `type component X { ... }`
// declaration in mod and returns the set of variable / timer / port
// names introduced inside. We use this for rule 16.1.4 (g): an
// assignment whose LHS is a component variable inside a restricted
// function counts as a side effect, because the component's snapshot
// is what the matcher is reading from.
func collectComponentVars(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		ct, ok := n.(*syntax.ComponentTypeDecl)
		if !ok {
			return true
		}
		if ct.Body == nil {
			return false
		}
		for _, s := range ct.Body.Stmts {
			ds, ok := s.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			if vd, ok := ds.Decl.(*syntax.ValueDecl); ok {
				for _, dec := range vd.Decls {
					if dec == nil || dec.Name == nil {
						continue
					}
					if dec.Name.Tok != nil {
						out[dec.Name.String()] = true
					}
				}
			}
		}
		return false
	})
	return out
}

// CheckSideEffectInRestrictedContext walks the module looking for the
// 16.1.4 violation pattern. It is invoked by Analyzer.Analyze.
func (a *Analyzer) checkSideEffectInRestrictedContext(mod *syntax.Module) []Diagnostic {
	compVars := collectComponentVars(mod)
	// Pass 1: collect every function declared in the module and the
	// set of forbidden selector names that appear inside its body. We
	// also track which function calls each function body makes, so we
	// can later compute the transitive closure.
	type funcInfo struct {
		decl       *syntax.FuncDecl
		direct     map[string]bool // forbidden op names directly in body
		calls      map[string]bool // names of in-module funcs it calls
		transitive map[string]bool // post-closure result
	}
	funcs := map[string]*funcInfo{}

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok {
			continue
		}
		name := syntax.Name(fn.Name)
		if name == "" {
			continue
		}
		info := &funcInfo{
			decl:   fn,
			direct: map[string]bool{},
			calls:  map[string]bool{},
		}
		// External function declarations have no body. ETSI 16.1.4
		// rule (e) classes them as side-effecting by default unless
		// the spec marks them `@deterministic`. The Modif token on
		// FuncDecl is precisely that annotation.
		if fn.External != nil && fn.Body == nil {
			if !isDeterministicModif(fn.Modif) {
				info.direct["external-nondeterministic"] = true
			}
		} else if fn.Body != nil {
			collectFnBodyOps(fn.Body, info.direct, info.calls)
			if assignsComponentVar(fn.Body, compVars) {
				info.direct["component-var-assignment"] = true
			}
		}
		// Rule 16.1.4 (c): a value-returning function with `out`
		// or `inout` formal parameters has observable side
		// effects through writeback; it cannot be used in a
		// restricted context regardless of the body.
		if hasOutOrInoutFormalPar(fn.Params) {
			info.direct["out-or-inout-formal-param"] = true
		}
		// Rule 16.1.4 (f): @fuzzy formal parameters or local
		// fuzzy variables introduce evaluation timing that
		// breaks snapshot consistency.
		if hasFuzzyFormalPar(fn.Params) {
			info.direct["fuzzy-formal-param"] = true
		}
		if fn.Body != nil && bodyDeclaresFuzzy(fn.Body) {
			info.direct["fuzzy-local-decl"] = true
		}
		funcs[name] = info
	}

	// Pass 2: transitive closure. Iterate until no new ops are added
	// (the call graph is small per module so this terminates fast).
	changed := true
	for changed {
		changed = false
		for _, fi := range funcs {
			if fi.transitive == nil {
				fi.transitive = map[string]bool{}
				for k := range fi.direct {
					fi.transitive[k] = true
				}
			}
			for callee := range fi.calls {
				other, ok := funcs[callee]
				if !ok {
					continue
				}
				src := other.transitive
				if src == nil {
					src = other.direct
				}
				for k := range src {
					if !fi.transitive[k] {
						fi.transitive[k] = true
						changed = true
					}
				}
			}
		}
	}

	// Pass 3: walk the module for restricted contexts and flag any
	// CallExpr whose callee has a non-empty transitive forbidden set.
	var diags []Diagnostic

	flagCall := func(call *syntax.CallExpr) {
		if call == nil || call.Fun == nil {
			return
		}
		id, ok := call.Fun.(*syntax.Ident)
		if !ok {
			return
		}
		fi, ok := funcs[id.String()]
		if !ok {
			return
		}
		if len(fi.transitive) == 0 {
			return
		}
		op := firstKey(fi.transitive)
		diags = append(diags, Diagnostic{
			Code:     "restricted-context-side-effect",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"function %q used in a restricted context performs %q which is forbidden by 16.1.4",
				id.String(), op),
			Node: call,
			Span: syntax.SpanOf(call),
		})
	}

	flagDirect := func(node syntax.Node, op string) {
		diags = append(diags, Diagnostic{
			Code:     "restricted-context-side-effect",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%q is forbidden in a restricted context (16.1.4 / 20.2)", op),
			Node: node,
			Span: syntax.SpanOf(node),
		})
	}

	inspectCallsIn := func(root syntax.Node) {
		if root == nil {
			return
		}
		syntax.Inspect(root, func(n syntax.Node) bool {
			if n == nil {
				return false
			}
			if ce, ok := n.(*syntax.CallExpr); ok {
				flagCall(ce)
			}
			// In a restricted context, even a direct
			// selector / bare ident reference to a forbidden
			// op counts (no function-call indirection
			// needed). `[t.running]`, `[GeneralComp.create !=
			// null]` and `[deactivate]` are the typical
			// patterns the conformance suite exercises.
			if sel, ok := n.(*syntax.SelectorExpr); ok {
				if sid, ok := sel.Sel.(*syntax.Ident); ok && sid.Tok != nil {
					if forbiddenSelectorSels[sid.String()] {
						flagDirect(sel, sid.String())
					}
				}
			}
			if id, ok := n.(*syntax.Ident); ok && id.Tok != nil {
				if forbiddenBareCallees[id.String()] {
					flagDirect(id, id.String())
				}
			}
			return true
		})
	}

	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			// Template RHS - any function call inside the
			// initializer expression is in a restricted ctx
			// because the template can be used in receive().
			inspectCallsIn(x.Value)
		case *syntax.FuncDecl:
			// Altstep local definition initializers are
			// restricted contexts per 16.2 (see 16.1.4 cross
			// reference). We walk every Declarator.Value in
			// the altstep's body.
			if x.KindTok != nil && x.KindTok.Kind() == syntax.ALTSTEP {
				if x.Body != nil {
					syntax.Inspect(x.Body, func(b syntax.Node) bool {
						if b == nil {
							return false
						}
						if vd, ok := b.(*syntax.ValueDecl); ok {
							for _, dec := range vd.Decls {
								if dec != nil && dec.Value != nil {
									inspectCallsIn(dec.Value)
								}
							}
						}
						return true
					})
				}
				// Default values for altstep formal
				// parameters are evaluated lazily at
				// each call and therefore subject to the
				// same restrictions per 16.2 (b).
				if x.Params != nil {
					for _, p := range x.Params.List {
						if p != nil && p.Value != nil {
							inspectCallsIn(p.Value)
						}
					}
				}
			}
		case *syntax.CommClause:
			// Alt branch guard `[expr]` is a restricted
			// context. The receive/trigger statement inside
			// Comm is handled by the *CallExpr case below.
			// However, if Comm is a direct call to an altstep
			// (i.e. `[] a_rcv(f_test());`), the altstep's
			// actual params are also restricted per 20.2.d.
			inspectCallsIn(x.X)
			if es, ok := x.Comm.(*syntax.ExprStmt); ok && es != nil {
				if ce, ok := es.Expr.(*syntax.CallExpr); ok && ce != nil {
					// Skip port comm ops - those are
					// handled separately so the
					// "callee not a known fn" warning
					// is not duplicated.
					if !isPortCommCall(ce) && ce.Args != nil {
						for _, a := range ce.Args.List {
							inspectCallsIn(a)
						}
					}
				}
			}
		case *syntax.CallExpr:
			// `port.receive(template)` - the args of a
			// receive-class selector call are restricted.
			if sel, ok := x.Fun.(*syntax.SelectorExpr); ok {
				if sid, ok := sel.Sel.(*syntax.Ident); ok {
					if restrictedReceiveSels[sid.String()] && x.Args != nil {
						for _, a := range x.Args.List {
							inspectCallsIn(a)
						}
					}
				}
			}
		}
		return true
	})

	return diags
}

// collectFnBodyOps walks a function body and fills `ops` with every
// forbidden selector seen and `calls` with every plain-ident callee.
func collectFnBodyOps(body syntax.Node, ops map[string]bool, calls map[string]bool) {
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		if sel, ok := n.(*syntax.SelectorExpr); ok {
			if sid, ok := sel.Sel.(*syntax.Ident); ok {
				name := sid.String()
				if forbiddenSelectorSels[name] {
					ops[name] = true
				}
			}
		}
		if ce, ok := n.(*syntax.CallExpr); ok {
			if id, ok := ce.Fun.(*syntax.Ident); ok {
				name := id.String()
				if forbiddenBareCallees[name] {
					ops[name] = true
				} else {
					calls[name] = true
				}
			}
		}
		if id, ok := n.(*syntax.Ident); ok && id.Tok != nil {
			name := id.String()
			// Bare keyword forms used as a statement: `stop;`
			// and friends register as Ident nodes outside any
			// SelectorExpr / CallExpr.
			if forbiddenBareCallees[name] {
				ops[name] = true
			}
		}
		return true
	})
}

func firstKey(m map[string]bool) string {
	for k := range m {
		return k
	}
	return ""
}

// assignsComponentVar reports whether body contains an assignment
// whose LHS resolves to a name in compVars. We only inspect plain
// identifiers and selectors rooted at the variable; an indirect
// assignment via alias may be missed, but the ETSI test pattern
// always writes the bare name on the left, e.g. `vc_int := 1`.
func assignsComponentVar(body syntax.Node, compVars map[string]bool) bool {
	if len(compVars) == 0 {
		return false
	}
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil || found {
			return false
		}
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		// Walk to the root identifier of the LHS.
		root := be.X
		for {
			switch lx := root.(type) {
			case *syntax.SelectorExpr:
				root = lx.X
				continue
			case *syntax.IndexExpr:
				root = lx.X
				continue
			}
			break
		}
		if id, ok := root.(*syntax.Ident); ok && id.Tok != nil {
			if compVars[id.String()] {
				found = true
			}
		}
		return true
	})
	return found
}

// isPortCommCall reports whether ce is a port communication
// operation (x.receive(...), x.trigger(...), etc.). Used so the
// CommClause walker can route those to inspectCallsIn over args
// directly rather than re-walking the surrounding call.
func isPortCommCall(ce *syntax.CallExpr) bool {
	if ce == nil || ce.Fun == nil {
		return false
	}
	sel, ok := ce.Fun.(*syntax.SelectorExpr)
	if !ok {
		return false
	}
	sid, ok := sel.Sel.(*syntax.Ident)
	if !ok || sid.Tok == nil {
		return false
	}
	return restrictedReceiveSels[sid.String()]
}

// hasFuzzyFormalPar reports whether any formal parameter is marked
// `@fuzzy` (rule 16.1.4 f).
func hasFuzzyFormalPar(params *syntax.FormalPars) bool {
	if params == nil {
		return false
	}
	for _, p := range params.List {
		if p == nil || p.Modif == nil {
			continue
		}
		if p.Modif.String() == "@fuzzy" {
			return true
		}
	}
	return false
}

// bodyDeclaresFuzzy reports whether body has a local var / template
// declaration tagged `@fuzzy`.
func bodyDeclaresFuzzy(body syntax.Node) bool {
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil || found {
			return false
		}
		if vd, ok := n.(*syntax.ValueDecl); ok && vd.Modif != nil {
			if vd.Modif.String() == "@fuzzy" {
				found = true
			}
		}
		if td, ok := n.(*syntax.TemplateDecl); ok && td.Modif != nil {
			if td.Modif.String() == "@fuzzy" {
				found = true
			}
		}
		return true
	})
	return found
}

// hasOutOrInoutFormalPar reports whether the formal parameter list
// has at least one `out` or `inout` parameter. The Direction token is
// either nil (= `in`), or one of the OUT / INOUT keywords.
func hasOutOrInoutFormalPar(params *syntax.FormalPars) bool {
	if params == nil {
		return false
	}
	for _, p := range params.List {
		if p == nil || p.Direction == nil {
			continue
		}
		switch p.Direction.Kind() {
		case syntax.OUT, syntax.INOUT:
			return true
		}
	}
	return false
}

// isDeterministicModif reports whether the @modif token on a FuncDecl
// is `@deterministic`. External functions marked deterministic are
// excluded from the 16.1.4 (e) restriction because the spec
// guarantees idempotent behaviour for the matcher.
func isDeterministicModif(tok syntax.Token) bool {
	if tok == nil {
		return false
	}
	return tok.String() == "@deterministic"
}
