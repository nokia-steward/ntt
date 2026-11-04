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
//     g() calling h() inherits h's forbidden ops - except having out
//     or inout parameters, which note 7 does not propagate).
//  2. Walking each module for "restricted contexts" - alt branch
//     guards, args to receive / trigger / check / getreply / getcall /
//     catch, altstep local definitions, the body of a template used in
//     one of those, and the body of a deterministic fuzzy template
//     (15.0 g) - and for every CallExpr in such a context emitting a
//     diagnostic if the callee has any forbidden op.
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
	lazyMembers := collectLazyOrFuzzyMembers(mod)
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
	outPars := collectByRefParams(mod)

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
			if assignsComponentVar(fn.Body, compVars) || passesComponentVarToOutPar(fn.Body, compVars, outPars) {
				info.direct["component-var-assignment"] = true
			}
		}
		// Rule 16.1.4 (j): a value-returning function with `out`
		// or `inout` formal parameters has observable side
		// effects through writeback; it cannot be used in a
		// restricted context regardless of the body. Note 7: the
		// restriction is not recursive, see outOrInoutOp.
		if hasOutOrInoutFormalPar(fn.Params) {
			info.direct[outOrInoutOp] = true
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
		// Rule 16.1.4 (m): referencing lazy or fuzzy variables,
		// parameters or templates not declared deterministic.
		if fn.Body != nil {
			names := map[string]bool{}
			for k := range lazyMembers {
				names[k] = true
			}
			if fn.Params != nil {
				for _, p := range fn.Params.List {
					if p != nil && p.Name != nil && lazyOrFuzzy(p.Modif, p.Deterministic) {
						names[p.Name.String()] = true
					}
				}
			}
			if referencesAny(fn.Body, names) {
				info.direct["lazy-or-fuzzy-reference"] = true
			}
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
					// Note 7: calling a function that itself
					// calls one with out or inout parameters
					// is legal.
					if k == outOrInoutOp {
						continue
					}
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

	// flagOutParCalls reports the calls in root of functions with out
	// or inout parameters, which the body of a fuzzy template shall
	// not contain (15.0 e). Functions called may use such functions
	// internally.
	flagOutParCalls := func(root syntax.Node) {
		if root == nil {
			return
		}
		syntax.Inspect(root, func(n syntax.Node) bool {
			if n == nil {
				return false
			}
			call, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			id, ok := call.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			if fi, ok := funcs[id.String()]; ok && fi.direct[outOrInoutOp] {
				diags = append(diags, Diagnostic{
					Code:     "restricted-context-side-effect",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"function %q with out or inout parameters is called in the body of a fuzzy template (15.0)",
						id.String()),
					Node: call,
					Span: syntax.SpanOf(call),
				})
			}
			return true
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

	// A template's body is evaluated where the template is used, so
	// it is a restricted context when the template is referenced
	// from one (16.1.4: "in templates, template fields"). A name is
	// the template of the behaviour it is written in, else the
	// module's. The body of a template of another module, which this
	// one imports, is not seen here.
	templates, ownerOf := collectTemplateDecls(mod)
	inspectedTemplates := map[*syntax.TemplateDecl]bool{}

	var inspectCallsIn func(root syntax.Node)
	inspectTemplate := func(td *syntax.TemplateDecl) {
		if inspectedTemplates[td] {
			return
		}
		inspectedTemplates[td] = true
		inspectCallsIn(td.Base)
		inspectCallsIn(td.Value)
	}

	inspectCallsIn = func(root syntax.Node) {
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
				if isTestcaseStop(sel) {
					return false
				}
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
				for _, td := range templates.named(id, ownerOf[id]) {
					inspectTemplate(td)
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
			// 15.0 (g): the body of a deterministic fuzzy
			// template is restricted wherever it is used.
			// Other templates are restricted only where they
			// are used in a restricted context (see
			// inspectTemplate), except that a fuzzy template
			// shall not call functions with out or inout
			// parameters directly (15.0 e).
			if isFuzzyModif(x.Modif) {
				if x.Deterministic != nil {
					inspectTemplate(x)
				} else {
					flagOutParCalls(x.Value)
				}
			}
		case *syntax.FuncDecl:
			// Altstep local definition initializers are
			// restricted contexts per 16.2 (b). The local
			// definitions are the declarations preceding the
			// alternatives; declarations inside a branch's
			// statement block are ordinary behaviour.
			if x.KindTok != nil && x.KindTok.Kind() == syntax.ALTSTEP {
				if x.Body != nil {
					for _, s := range x.Body.Stmts {
						ds, ok := s.(*syntax.DeclStmt)
						if !ok {
							continue
						}
						switch d := ds.Decl.(type) {
						case *syntax.ValueDecl:
							for _, dec := range d.Decls {
								if dec != nil && dec.Value != nil {
									inspectCallsIn(dec.Value)
								}
							}
						case *syntax.TemplateDecl:
							inspectTemplate(d)
						}
					}
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

	return dedupeDiagnostics(diags)
}

// collectFnBodyOps walks a function body and fills `ops` with every
// forbidden selector seen and `calls` with every plain-ident callee.
func collectFnBodyOps(body syntax.Node, ops map[string]bool, calls map[string]bool) {
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return false
		}
		if sel, ok := n.(*syntax.SelectorExpr); ok {
			// testcase.stop may be invoked from specific
			// places (clause 18, table 16).
			if isTestcaseStop(sel) {
				return false
			}
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

// outOrInoutOp is the forbidden-op key of a function with out or inout
// formal parameters (16.1.4 j). Unlike the other keys it is not
// inherited by callers (note 7).
const outOrInoutOp = "out-or-inout-formal-param"

// isTestcaseStop reports whether sel is the `testcase.stop` operation.
func isTestcaseStop(sel *syntax.SelectorExpr) bool {
	id, ok := sel.X.(*syntax.Ident)
	return ok && id.Tok != nil && id.Tok.Kind() == syntax.TESTCASE
}

// isFuzzyModif reports whether the modifier token is `@fuzzy`.
func isFuzzyModif(tok syntax.Token) bool {
	return tok != nil && tok.String() == "@fuzzy"
}

// templateDecls are the templates of a module by name: those of the
// module, in groups too, and those local to each of its behaviours.
type templateDecls struct {
	module map[string][]*syntax.TemplateDecl
	local  map[*syntax.FuncDecl]map[string][]*syntax.TemplateDecl
}

// named is what a reference id, written in behaviour owner (nil at
// module level), names: the behaviour's template of that name, else the
// module's.
func (t templateDecls) named(id *syntax.Ident, owner *syntax.FuncDecl) []*syntax.TemplateDecl {
	if tds := t.local[owner][id.String()]; owner != nil && len(tds) > 0 {
		return tds
	}
	return t.module[id.String()]
}

// collectTemplateDecls collects the templateDecls of mod, and the
// behaviour each identifier named like a template is written in.
func collectTemplateDecls(mod *syntax.Module) (templateDecls, map[*syntax.Ident]*syntax.FuncDecl) {
	t := templateDecls{module: map[string][]*syntax.TemplateDecl{}, local: map[*syntax.FuncDecl]map[string][]*syntax.TemplateDecl{}}
	var fns []*syntax.FuncDecl
	var walk func(defs []*syntax.ModuleDef)
	walk = func(defs []*syntax.ModuleDef) {
		for _, d := range defs {
			if d == nil {
				continue
			}
			switch x := d.Def.(type) {
			case *syntax.GroupDecl:
				walk(x.Defs)
			case *syntax.TemplateDecl:
				if x.Name != nil {
					t.module[x.Name.String()] = append(t.module[x.Name.String()], x)
				}
			case *syntax.FuncDecl:
				fns = append(fns, x)
			}
		}
	}
	walk(mod.Defs)
	names := map[string]bool{}
	for name := range t.module {
		names[name] = true
	}
	for _, fn := range fns {
		if fn.Body == nil {
			continue
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			if td, ok := n.(*syntax.TemplateDecl); ok && td.Name != nil {
				if t.local[fn] == nil {
					t.local[fn] = map[string][]*syntax.TemplateDecl{}
				}
				t.local[fn][td.Name.String()] = append(t.local[fn][td.Name.String()], td)
				names[td.Name.String()] = true
			}
			return n != nil
		})
	}
	ownerOf := map[*syntax.Ident]*syntax.FuncDecl{}
	for _, fn := range fns {
		syntax.Inspect(fn, func(n syntax.Node) bool {
			if id, ok := n.(*syntax.Ident); ok && names[id.String()] {
				ownerOf[id] = fn
			}
			return n != nil
		})
	}
	return t, ownerOf
}

// dedupeDiagnostics drops a diagnostic of a code a node already has: a
// call two of a rule's checks find is reported once.
func dedupeDiagnostics(diags []Diagnostic) []Diagnostic {
	type key struct {
		node syntax.Node
		code string
	}
	seen := map[key]bool{}
	out := diags[:0]
	for _, d := range diags {
		k := key{d.Node, d.Code}
		if d.Node != nil && seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	return out
}

// passesComponentVarToOutPar reports whether body passes a component
// variable as the actual parameter of an out or inout formal parameter,
// which changes the variable like an assignment does (16.1.4 g).
func passesComponentVarToOutPar(body syntax.Node, compVars map[string]bool, outPars byRefParams) bool {
	if len(compVars) == 0 || len(outPars) == 0 {
		return false
	}
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil || found {
			return false
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce.Args == nil {
			return true
		}
		for _, a := range outPars.args(ce) {
			if compVars[rootIdentName(a)] {
				found = true
			}
		}
		return true
	})
	return found
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

// lazyOrFuzzy reports whether a declaration with the modifiers modif and
// deterministic is lazy or fuzzy and not declared deterministic.
func lazyOrFuzzy(modif, deterministic syntax.Token) bool {
	if modif == nil || deterministic != nil {
		return false
	}
	switch modif.String() {
	case "@lazy", "@fuzzy":
		return true
	}
	return false
}

// collectLazyOrFuzzyMembers returns the names of the component members
// and module definitions of mod, variables and templates, that are lazy
// or fuzzy and not declared deterministic (rule 16.1.4 m).
func collectLazyOrFuzzyMembers(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	add := func(d syntax.Node) {
		switch x := d.(type) {
		case *syntax.ValueDecl:
			if lazyOrFuzzy(x.Modif, x.Deterministic) {
				for _, dec := range x.Decls {
					if dec != nil && dec.Name != nil && dec.Name.Tok != nil {
						out[dec.Name.String()] = true
					}
				}
			}
		case *syntax.TemplateDecl:
			if x.Name != nil && lazyOrFuzzy(x.Modif, x.Deterministic) {
				out[x.Name.String()] = true
			}
		}
	}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		add(d.Def)
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Body == nil {
			continue
		}
		for _, st := range ct.Body.Stmts {
			if ds, ok := st.(*syntax.DeclStmt); ok {
				add(ds.Decl)
			}
		}
	}
	return out
}

// referencesAny reports whether body refers to one of names, other than
// by declaring it.
func referencesAny(body syntax.Node, names map[string]bool) bool {
	if len(names) == 0 {
		return false
	}
	declared := map[*syntax.Ident]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if d, ok := n.(*syntax.Declarator); ok && d.Name != nil {
			declared[d.Name] = true
		}
		return n != nil
	})
	found := false
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil || found {
			return false
		}
		if id, ok := n.(*syntax.Ident); ok && id.Tok != nil && !declared[id] && names[id.String()] {
			found = true
		}
		return true
	})
	return found
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
