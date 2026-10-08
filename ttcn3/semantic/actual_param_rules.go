// actual_param_rules.go enforces the per-argument template
// restrictions described in ETSI ES 201 873-1 clause 5.4.2 + 15.8.
//
// Each formal parameter carries a restriction (template(value),
// template(present), template(omit) or `omit T` shorthand). Any
// actual argument passed in a call must obey that restriction:
//
//   - `template(value)`  / non-template value param: a concrete
//     value only - no omit, no matching mechanism (wildcards,
//     ranges, value-lists, patterns, length, permutation,
//     complement, subset, superset).
//   - `template(omit)`   / `omit T` shorthand: value or `omit`
//     only - no matching mechanism.
//   - `template(present)`: value or matcher, but never `omit`
//     and never `*` (AnyValueOrNone).
//   - plain `template T`: anything goes.
//
// The check only fires on positional arguments whose shape is a
// literal or paren expression - it does not try to evaluate
// identifiers, so a template reference passed by name is always
// considered safe. That is on purpose: the conformance fixtures
// that we are trying to catch all pass a literal matching
// construct directly in the call site.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// formalSig captures the subset of a callable signature we need
// for actual-parameter validation: the restriction tag and the
// parameter name (used in diagnostics).
type formalSig struct {
	name string
	// restriction is the canonical restriction string:
	// "value", "present", "omit", or "" for none.
	restriction string
	// hasTemplate is true if the parameter is declared with the
	// `template` keyword (possibly with parens). When false and
	// restriction == "omit", the parameter is the `omit T`
	// shorthand which accepts value or `omit` but no matcher.
	hasTemplate bool
	// modif is the parameter modifier, one of "" / "@lazy" /
	// "@fuzzy". Used by the lazy/fuzzy side-effect rule.
	modif string
	// modif2 is the second parameter modifier (e.g.
	// `@deterministic` in `@fuzzy @deterministic`). Used to
	// detect the deterministic-fuzzy combination so calls to
	// such formals can be restricted to side-effect-free
	// arguments per ETSI 16.1.4.
	modif2 string
	// dir is the parameter direction surface name, one of
	// "" / "in" / "out" / "inout".
	dir string
	// typeName is the bare named-type identifier of the formal,
	// or "" when the type is anonymous / unsupported.
	typeName string
}

// checkActualParameterRules walks every CallExpr in executable
// bodies and validates each positional actual argument against
// the corresponding formal parameter restriction.
func (a *Analyzer) checkActualParameterRules(mod *syntax.Module) []Diagnostic {
	sigs := collectModuleSigs(mod)
	if len(sigs) == 0 {
		return nil
	}
	lazyVars := collectLazyFuzzyVars(mod)
	paramTmpls := collectParameterizedTemplates(mod)
	portTypes := collectPortTypeNames(mod)
	localVarTypes := collectLocalVarTypeNames(mod)
	tmplVarRestr := collectTemplateVarRestrictions(mod)
	tmplBodies := collectActualParamTemplateBodies(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil {
			return true
		}
		callee := identName(ce.Fun)
		if callee == "" {
			return true
		}
		params, ok := sigs[callee]
		if !ok {
			return true
		}
		args := ce.Args.List
		for i, arg := range args {
			if i >= len(params) {
				break
			}
			p := params[i]
			if v := violationForArg(arg, p); v != "" {
				diags = append(diags, Diagnostic{
					Code:     "actual-parameter-restriction-violation",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"argument %d of %s: %s is not allowed for %s",
						i+1, callee, v, describeRestriction(p)),
					Node: arg,
					Span: syntax.SpanOf(arg),
				})
			}
			if p.hasTemplate {
				if reason := forbiddenTemplateFieldActual(arg, tmplBodies); reason != "" {
					diags = append(diags, Diagnostic{
						Code:     "actual-parameter-template-field-reference",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"argument %d of %s references %s, which is not a valid actual parameter field reference (ETSI 5.4.2 / 15.6.2)",
							i+1, callee, reason),
						Node: arg,
						Span: syntax.SpanOf(arg),
					})
				}
			}
			if (p.modif == "@lazy" || p.modif == "@fuzzy") &&
				p.dir != "out" && p.dir != "inout" {
				if reason := lazyArgViolation(arg, sigs); reason != "" {
					diags = append(diags, Diagnostic{
						Code:     "lazy-fuzzy-arg-side-effect",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"argument %d of %s: %s formal parameter %q cannot accept a value derived from %s",
							i+1, callee, p.modif, p.name, reason),
						Node: arg,
						Span: syntax.SpanOf(arg),
					})
				}
			}
			if name := identName(arg); name != "" && paramTmpls[name] {
				diags = append(diags, Diagnostic{
					Code:     "parameterized-template-without-args",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"argument %d of %s: %q is a parameterized template and must be invoked with its own parameter list",
						i+1, callee, name),
					Node: arg,
					Span: syntax.SpanOf(arg),
				})
			}
			if (p.dir == "out" || p.dir == "inout") && len(lazyVars) > 0 {
				if modif := lazyFuzzyVarRef(arg, lazyVars); modif != "" {
					diags = append(diags, Diagnostic{
						Code:     "lazy-fuzzy-var-to-out-inout",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"argument %d of %s: cannot pass a %s variable to %s formal parameter %q",
							i+1, callee, modif, p.dir, p.name),
						Node: arg,
						Span: syntax.SpanOf(arg),
					})
				}
			}
			if p.dir == "out" || p.dir == "inout" {
				if base := stringElementBase(arg, localVarTypes); base != "" {
					diags = append(diags, Diagnostic{
						Code:     "string-element-to-out-inout",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"argument %d of %s: cannot pass string-element %s[...] to %s formal parameter %q (ETSI 5.4.2)",
							i+1, callee, base, p.dir, p.name),
						Node: arg,
						Span: syntax.SpanOf(arg),
					})
				}
			}
			if portTypes[p.typeName] {
				if why := nonPortArgReason(arg, portTypes, localVarTypes); why != "" {
					diags = append(diags, Diagnostic{
						Code:     "port-parameter-non-port-arg",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"argument %d of %s: port-typed parameter %q cannot accept %s",
							i+1, callee, p.name, why),
						Node: arg,
						Span: syntax.SpanOf(arg),
					})
				}
			}
			// ETSI 5.4.2 + 16.1.4: actual parameters passed to
			// `@fuzzy @deterministic` (or `@lazy
			// @deterministic`) formals must be side-effect
			// free per 16.1.4. We surface a violation when the
			// actual is a CallExpr to a known function whose
			// body uses one of the operations listed in
			// 16.1.4. Templates and constants slip through -
			// the heuristic only fires on direct call args.
			if p.isDeterministic() && len(p.modif) > 0 {
				if reason := deterministicArgViolation(arg, mod); reason != "" {
					diags = append(diags, Diagnostic{
						Code:     "deterministic-arg-side-effect",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"argument %d of %s: %s %s formal parameter %q cannot accept %s (ETSI 16.1.4)",
							i+1, callee, p.modif, p.modif2, p.name, reason),
						Node: arg,
						Span: syntax.SpanOf(arg),
					})
				}
			}

			// ETSI 5.4.2: for inout formal template parameters
			// the actual and formal restrictions must match
			// exactly. We only fire when the actual is a bare
			// identifier we have a declared restriction for -
			// expressions and inline template literals are
			// already handled by violationForArg above.
			if p.dir == "inout" && p.hasTemplate {
				if name := identName(arg); name != "" {
					if actualRestr, known := tmplVarRestr[name]; known &&
						actualRestr != p.restriction {
						diags = append(diags, Diagnostic{
							Code:     "inout-template-restriction-mismatch",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"argument %d of %s: inout template restriction `template(%s)` of formal %q does not match actual `template(%s)` of %q (ETSI 5.4.2)",
								i+1, callee, p.restriction, p.name, actualRestr, name),
							Node: arg,
							Span: syntax.SpanOf(arg),
						})
					}
				}
			}
		}
		return true
	})
	diags = append(diags, checkUnboundArgPerFunc(mod, sigs)...)
	return diags
}

// checkUnboundArgPerFunc walks every FuncDecl, collects its
// statically-unbound local variables, and rejects call sites that
// pass such a variable to a non-template `in` / `inout` formal.
// Doing this per-function avoids cross-scope shadowing where a
// var with the same name is bound in one function but unbound in
// another.
func checkUnboundArgPerFunc(mod *syntax.Module, sigs map[string][]formalSig) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		unbound := localUnboundVars(fn.Body)
		if len(unbound) == 0 {
			return false
		}
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			ce, ok := sn.(*syntax.CallExpr)
			if !ok || ce == nil || ce.Args == nil {
				return true
			}
			callee := identName(ce.Fun)
			if callee == "" {
				return true
			}
			params, ok := sigs[callee]
			if !ok {
				return true
			}
			for i, arg := range ce.Args.List {
				if i >= len(params) {
					break
				}
				p := params[i]
				if p.hasTemplate {
					continue
				}
				if p.dir != "" && p.dir != "in" && p.dir != "inout" {
					continue
				}
				name := identName(arg)
				if name == "" || !unbound[name] {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "uninitialised-arg-to-in-inout",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"argument %d of %s: variable %q is statically unbound and cannot be passed to %s parameter %q",
						i+1, callee, name, paramDirLabel(p.dir), p.name),
					Node: arg,
					Span: syntax.SpanOf(arg),
				})
			}
			return true
		})
		return false
	})
	return diags
}

// localUnboundVars returns the names of vars declared without an
// initialiser in body, minus any that are reassigned via `x := y`
// or passed to any call (conservatively assumed to bind them).
func localUnboundVars(body syntax.Node) map[string]bool {
	cand := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value != nil {
				continue
			}
			cand[dec.Name.String()] = true
		}
		return true
	})
	if len(cand) == 0 {
		return cand
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.BinaryExpr:
			if x.Op != nil && x.Op.Kind() == syntax.ASSIGN {
				if name := rootIdentName(x.X); name != "" {
					delete(cand, name)
				}
			}
		case *syntax.RedirectExpr:
			// `... -> value v` binds v at the time of the
			// receive/getreply, so v is no longer unbound.
			for _, vt := range x.Value {
				if name := rootIdentName(vt); name != "" {
					delete(cand, name)
				}
			}
			if name := identName(x.Sender); name != "" {
				delete(cand, name)
			}
		}
		return true
	})
	return cand
}

// rootIdentName strips Selector / Index / Paren wrappers and
// returns the bare ident at the root of e, or "" if none.
func rootIdentName(e syntax.Expr) string {
	for {
		if e == nil {
			return ""
		}
		switch x := e.(type) {
		case *syntax.Ident:
			return x.String()
		case *syntax.SelectorExpr:
			e = x.X
		case *syntax.IndexExpr:
			e = x.X
		case *syntax.ParenExpr:
			if len(x.List) != 1 {
				return ""
			}
			e = x.List[0]
		default:
			return ""
		}
	}
}

// collectPortTypeNames returns the set of named port types
// declared in the module (via `type port Name ...`).
func collectPortTypeNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		pd, ok := n.(*syntax.PortTypeDecl)
		if !ok || pd == nil || pd.Name == nil {
			return true
		}
		out[pd.Name.String()] = true
		return true
	})
	return out
}

// collectLocalVarTypeNames walks every ValueDecl in the module
// and records the declared type name per variable. We only keep
// the bare named-type identifier; anonymous and parameterised
// types resolve to "".
func collectLocalVarTypeNames(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := identName(vd.Type)
		if typeName == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = typeName
		}
		return true
	})
	return out
}

func paramDirLabel(dir string) string {
	switch dir {
	case "", "in":
		return "in"
	case "inout":
		return "inout"
	}
	return dir
}

// stringElementBase returns the base variable name when arg is
// an IndexExpr `var[index]` whose base variable was declared with
// a string-family type (charstring / universal charstring /
// bitstring / hexstring / octetstring). Anything else returns "".
func stringElementBase(arg syntax.Expr, varTypes map[string]string) string {
	ie, ok := arg.(*syntax.IndexExpr)
	if !ok || ie == nil {
		return ""
	}
	name := identName(ie.X)
	if name == "" {
		return ""
	}
	switch varTypes[name] {
	case "charstring", "universal charstring",
		"bitstring", "hexstring", "octetstring":
		return name
	}
	return ""
}

// nonPortArgReason returns a non-empty description when arg
// cannot legally be passed to a port-typed formal parameter:
//   - literal value (integers, strings, omit ...) - never a port
//   - bare ident referring to a non-port-typed local variable
//
// References to known port-typed locals, component-port selectors
// (`self.p`) and anything we cannot resolve fall through.
func nonPortArgReason(arg syntax.Expr, portTypes map[string]bool, varTypes map[string]string) string {
	if arg == nil {
		return ""
	}
	if lit, ok := arg.(*syntax.ValueLiteral); ok {
		// `-` is the explicit "skip" placeholder for `out`
		// formals; it leaves the actual unchanged and is
		// legal for any direction (5.4.1.2).
		if lit != nil && lit.Tok != nil && lit.Tok.String() == "-" {
			return ""
		}
		return "a literal value"
	}
	if name := identName(arg); name != "" {
		if t, ok := varTypes[name]; ok && !portTypes[t] {
			return fmt.Sprintf("variable %q of non-port type %q", name, t)
		}
	}
	return ""
}

// collectLazyFuzzyVars walks every ValueDecl in the module and
// returns a name -> modifier ("@lazy" / "@fuzzy") map for variables
// declared with one of those modifiers.
func collectLazyFuzzyVars(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.Modif == nil {
			return true
		}
		modif := vd.Modif.String()
		if modif != "@lazy" && modif != "@fuzzy" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = modif
		}
		return true
	})
	return out
}

// lazyFuzzyVarRef returns the modifier string ("@lazy" / "@fuzzy")
// when arg is a bare reference to a known lazy/fuzzy variable.
// Any non-Ident expression falls through.
func lazyFuzzyVarRef(arg syntax.Expr, lazyVars map[string]string) string {
	name := identName(arg)
	if name == "" {
		return ""
	}
	return lazyVars[name]
}

// isDeterministic reports whether the formal carries any modifier
// combination that includes `@deterministic`. ETSI 16.1.4 forbids
// side-effecting operations inside any function value bound to
// such a parameter.
func (s formalSig) isDeterministic() bool {
	return s.modif == "@deterministic" || s.modif2 == "@deterministic"
}

// deterministicArgViolation walks arg looking for direct CallExpr
// references to functions whose body uses one of the operations
// banned by ETSI 16.1.4 inside a deterministic context. Returns a
// human-readable reason or "" when no violation is found.
func deterministicArgViolation(arg syntax.Expr, mod *syntax.Module) string {
	ce, ok := arg.(*syntax.CallExpr)
	if !ok || ce == nil {
		return ""
	}
	callee := identName(ce.Fun)
	if callee == "" {
		return ""
	}
	body, params := funcBodyAndParamsByName(mod, callee)
	if body == nil {
		return ""
	}
	if reason := bodyDeterministicViolation(body); reason != "" {
		return fmt.Sprintf("a call to %s() which %s", callee, reason)
	}
	lazy := collectLazyFuzzyVars(mod)
	// Also surface `@lazy` / `@fuzzy` parameters of the callee
	// itself - referencing them in the body is the same kind
	// of restriction violation per ETSI 16.1.4 m.
	for _, p := range params {
		if p == nil || p.Name == nil || p.Modif == nil {
			continue
		}
		mtag := p.Modif.String()
		if mtag != "@lazy" && mtag != "@fuzzy" {
			continue
		}
		lazy[p.Name.String()] = mtag
	}
	if reason := bodyLazyFuzzyRefViolation(body, lazy); reason != "" {
		return fmt.Sprintf("a call to %s() which %s", callee, reason)
	}
	return ""
}

// bodyLazyFuzzyRefViolation walks body looking for any Ident that
// references a `@lazy`/`@fuzzy` variable. Such references are
// banned inside the evaluation of a `@deterministic` parameter
// per ETSI 16.1.4 m unless the source itself is `@deterministic`.
// We deliberately ignore the cross-deterministic case for now -
// the variable scope tracker doesn't tag the second modifier.
func bodyLazyFuzzyRefViolation(body *syntax.BlockStmt, lazy map[string]string) string {
	var reason string
	if len(lazy) == 0 {
		return ""
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if reason != "" {
			return false
		}
		id, ok := n.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			return true
		}
		if modif, found := lazy[id.String()]; found {
			reason = "references " + modif + " variable " +
				id.String() + " (ETSI 16.1.4 m)"
			return false
		}
		return true
	})
	return reason
}

// funcBodyAndParamsByName returns the BlockStmt body and formal
// parameter list of the first FuncDecl in mod whose name matches
// the requested identifier, or (nil, nil).
func funcBodyAndParamsByName(mod *syntax.Module, name string) (*syntax.BlockStmt, []*syntax.FormalPar) {
	var (
		body   *syntax.BlockStmt
		params []*syntax.FormalPar
	)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if body != nil {
			return false
		}
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Name == nil || fn.Body == nil {
			return true
		}
		if fn.Name.String() == name {
			body = fn.Body
			if fn.Params != nil {
				params = fn.Params.List
			}
			return false
		}
		return true
	})
	return body, params
}

// bodyDeterministicViolation walks fn's body looking for the
// operations that ETSI 16.1.4 forbids: component lifecycle, timer
// ops, port comm ops, action, checkstate, check, raise/getreply
// and references through `self` / `mtc`. Returns a short reason
// describing the first violation found, or "".
func bodyDeterministicViolation(body *syntax.BlockStmt) string {
	var reason string
	classify := func(op string) string {
		switch op {
		case "start", "stop", "kill", "done", "killed",
			"running", "alive", "create":
			return "uses a component lifecycle op " + op +
				" (ETSI 16.1.4 a)"
		case "timeout", "read":
			return "uses the timer op " + op + " (ETSI 16.1.4 d)"
		case "send", "receive", "trigger", "raise", "getcall",
			"getreply", "reply", "catch", "checkstate",
			"halt", "clear", "call", "connect", "disconnect",
			"map", "unmap":
			return "uses the port op " + op +
				" (ETSI 16.1.4 b/c/e)"
		case "action":
			return "uses the action op (ETSI 16.1.4 c)"
		case "check":
			return "uses the check op (ETSI 16.1.4 f)"
		case "setverdict":
			return "uses the setverdict op (ETSI 16.1.4 i)"
		case "activate", "deactivate":
			return "uses the " + op +
				" op (ETSI 16.1.4 h)"
		case "rnd":
			return "uses the predefined rnd function (ETSI 16.1.4 j)"
		case "setencode":
			return "uses the setencode op (ETSI 16.1.4 k)"
		}
		return ""
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if reason != "" {
			return false
		}
		switch x := n.(type) {
		case *syntax.SelectorExpr:
			if x.Sel == nil {
				return true
			}
			id, ok := x.Sel.(*syntax.Ident)
			if !ok || id == nil || id.Tok == nil {
				return true
			}
			if r := classify(id.String()); r != "" {
				reason = r
			}
		case *syntax.CallExpr:
			id, ok := x.Fun.(*syntax.Ident)
			if !ok || id == nil || id.Tok == nil {
				return true
			}
			if r := classify(id.String()); r != "" {
				reason = r
			}
		case *syntax.ExprStmt:
			id, ok := x.Expr.(*syntax.Ident)
			if !ok || id == nil || id.Tok == nil {
				return true
			}
			if r := classify(id.String()); r != "" {
				reason = r
			}
		}
		return true
	})
	return reason
}

// lazyArgViolation returns a non-empty reason when arg contains
// a CallExpr to a function with out / inout parameters. The lazy
// / fuzzy formal may evaluate the expression zero or many times,
// which would lose or duplicate those side effects.
func lazyArgViolation(arg syntax.Expr, sigs map[string][]formalSig) string {
	var reason string
	syntax.Inspect(arg, func(n syntax.Node) bool {
		if reason != "" {
			return false
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		callee := identName(ce.Fun)
		if callee == "" {
			return true
		}
		params, ok := sigs[callee]
		if !ok {
			return true
		}
		for _, fp := range params {
			if fp.dir == "out" || fp.dir == "inout" {
				reason = fmt.Sprintf("a call to %s() whose parameter %q has direction %s", callee, fp.name, fp.dir)
				return false
			}
		}
		return true
	})
	return reason
}

// collectModuleSigs walks the module and returns a map from
// function / altstep / testcase / template name to the ordered
// list of formal parameter signatures we care about. Templates
// with an empty FormalPars are still recorded (with len 0) so
// the parameterized-without-args check can distinguish "no
// parameters" from "not a template at all".
func collectModuleSigs(mod *syntax.Module) map[string][]formalSig {
	out := map[string][]formalSig{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.FuncDecl:
			if x == nil || x.Name == nil || x.Params == nil {
				return true
			}
			out[x.Name.String()] = paramsToSigs(x.Params.List)
		case *syntax.TemplateDecl:
			if x == nil || x.Name == nil {
				return true
			}
			if x.Params == nil {
				out[x.Name.String()] = nil
				return true
			}
			out[x.Name.String()] = paramsToSigs(x.Params.List)
		}
		return true
	})
	return out
}

// collectTemplateVarRestrictions returns a map from variable name
// to its declared template restriction tag ("value", "present",
// "omit", or ""), for every `var template[(R)] T name` declaration
// in the module. Used to detect inout template restriction
// mismatches at call sites (ETSI 5.4.2).
func collectTemplateVarRestrictions(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			return true
		}
		if vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction == nil {
			return true
		}
		restr := ""
		if vd.TemplateRestriction.Tok != nil {
			restr = vd.TemplateRestriction.Tok.String()
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			if _, dup := out[d.Name.String()]; !dup {
				out[d.Name.String()] = restr
			}
		}
		return true
	})
	return out
}

// collectActualParamTemplateBodies records the concrete bodies of
// non-parameterised templates and initialised template variables.
// Actual-parameter reference checks use this to apply the same
// field-reference restrictions that would apply on the RHS/LHS of
// assignments (ETSI 5.4.2 delegates to 15.6.2).
func collectActualParamTemplateBodies(mod *syntax.Module) map[string]syntax.Expr {
	out := map[string]syntax.Expr{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			if x == nil || x.Name == nil || x.Value == nil {
				return true
			}
			if x.Params != nil && len(x.Params.List) > 0 {
				return true
			}
			out[x.Name.String()] = x.Value
		case *syntax.ValueDecl:
			if x == nil || x.TemplateRestriction == nil {
				return true
			}
			for _, d := range x.Decls {
				if d == nil || d.Name == nil || d.Value == nil {
					continue
				}
				out[d.Name.String()] = d.Value
			}
		}
		return true
	})
	return out
}

func forbiddenTemplateFieldActual(arg syntax.Expr, bodies map[string]syntax.Expr) string {
	if len(bodies) == 0 {
		return ""
	}
	path := actualParamSelectorPath(arg)
	if len(path) < 3 {
		return ""
	}
	body := bodies[path[0]]
	if body == nil {
		return ""
	}
	field := path[1]
	value := compositeFieldValue(body, field)
	if value == nil {
		return ""
	}
	if label := invalidFieldReferenceValue(value); label != "" {
		return fmt.Sprintf("template field `%s.%s` containing %s", path[0], field, label)
	}
	return ""
}

func actualParamSelectorPath(e syntax.Expr) []string {
	switch x := e.(type) {
	case *syntax.Ident:
		if x == nil {
			return nil
		}
		return []string{x.String()}
	case *syntax.SelectorExpr:
		if x == nil {
			return nil
		}
		base := actualParamSelectorPath(x.X)
		if len(base) == 0 {
			return nil
		}
		sel, ok := x.Sel.(*syntax.Ident)
		if !ok || sel == nil {
			return nil
		}
		return append(base, sel.String())
	case *syntax.ParenExpr:
		if x == nil || len(x.List) != 1 {
			return nil
		}
		return actualParamSelectorPath(x.List[0])
	}
	return nil
}

func compositeFieldValue(body syntax.Expr, field string) syntax.Expr {
	cl, ok := body.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return nil
	}
	for _, item := range cl.List {
		be, ok := item.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			continue
		}
		if identName(be.X) == field {
			return be.Y
		}
	}
	return nil
}

func invalidFieldReferenceValue(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x == nil || x.Tok == nil {
			return ""
		}
		switch x.Tok.Kind() {
		case syntax.MUL:
			return "`*`"
		case syntax.OMIT:
			return "`omit`"
		}
	case *syntax.ParenExpr:
		if x != nil && len(x.List) > 1 {
			return "a value list"
		}
	case *syntax.CallExpr:
		if id, ok := x.Fun.(*syntax.Ident); ok && id != nil {
			switch id.String() {
			case "permutation", "complement", "subset", "superset":
				return id.String()
			}
		}
	case *syntax.LengthExpr:
		return "a length-restricted template"
	case *syntax.PatternExpr:
		return "a pattern"
	}
	return ""
}

// collectParameterizedTemplates returns the set of TemplateDecl
// names that declare formal parameters.
func collectParameterizedTemplates(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil || td.Params == nil {
			return true
		}
		if len(td.Params.List) > 0 {
			out[td.Name.String()] = true
		}
		return true
	})
	return out
}

func paramsToSigs(list []*syntax.FormalPar) []formalSig {
	out := make([]formalSig, 0, len(list))
	for _, p := range list {
		if p == nil {
			continue
		}
		sig := formalSig{}
		if p.Name != nil {
			sig.name = p.Name.String()
		}
		if rs := p.TemplateRestriction; rs != nil {
			if rs.TemplateTok != nil && rs.TemplateTok.Kind() != syntax.ILLEGAL {
				sig.hasTemplate = true
			}
			if rs.Tok != nil && rs.Tok.Kind() != syntax.ILLEGAL {
				sig.restriction = rs.Tok.String()
			}
		}
		if p.Modif != nil {
			sig.modif = p.Modif.String()
		}
		if p.Modif2 != nil {
			sig.modif2 = p.Modif2.String()
		}
		if p.Direction != nil {
			sig.dir = p.Direction.String()
		}
		if p.Type != nil {
			sig.typeName = identName(p.Type)
		}
		out = append(out, sig)
	}
	return out
}

// describeRestriction returns a human-readable phrase used in
// diagnostic messages.
func describeRestriction(p formalSig) string {
	switch {
	case p.hasTemplate && p.restriction == "value":
		return "template(value) parameter"
	case p.hasTemplate && p.restriction == "present":
		return "template(present) parameter"
	case p.hasTemplate && p.restriction == "omit":
		return "template(omit) parameter"
	case !p.hasTemplate && p.restriction == "omit":
		return "omit value parameter"
	case !p.hasTemplate && p.restriction == "":
		return "value parameter"
	}
	return "parameter"
}

// violationForArg classifies arg against the formal restriction
// and returns a non-empty description string if the argument
// breaks the restriction.
func violationForArg(arg syntax.Expr, p formalSig) string {
	if arg == nil {
		return ""
	}
	// Plain template (no restriction tag) or restriction-less
	// value parameter that is also not a template: nothing to
	// enforce statically at the call site.
	if p.hasTemplate && p.restriction == "" {
		return ""
	}

	// `template(value)` / `value` parameter:
	//   - reject any matcher / paren-list / omit / wildcard
	// `template(omit)` or `omit T` parameter:
	//   - reject any matcher / paren-list / wildcard
	//     (omit itself is allowed)
	// `template(present)` parameter:
	//   - reject only `omit` and `*` (AnyValueOrNone)
	switch p.restriction {
	case "value", "":
		// "" + no template means plain value param.
		return forbiddenMatcher(arg, true /*rejectOmit*/, true /*rejectAnyOrNone*/)
	case "omit":
		return forbiddenMatcher(arg, false /*omit ok*/, true /*reject *  */)
	case "present":
		// Only omit / * are forbidden; matchers are allowed.
		if isOmitTok(arg) {
			return "omit"
		}
		if isAnyOrNoneTok(arg) {
			return "*"
		}
		return ""
	}
	return ""
}

// forbiddenMatcher returns a short label describing the matching
// construct found in arg, or "" if arg is value-shaped.
func forbiddenMatcher(arg syntax.Expr, rejectOmit, rejectAnyOrNone bool) string {
	if v, ok := arg.(*syntax.ValueLiteral); ok && v != nil && v.Tok != nil {
		switch v.Tok.Kind() {
		case syntax.OMIT:
			if rejectOmit {
				return "omit"
			}
			return ""
		case syntax.ANY:
			return "?"
		case syntax.MUL:
			if rejectAnyOrNone {
				return "*"
			}
			return ""
		}
		return ""
	}
	// Paren-wrapped: either a value-list `(a, b, c)` or a
	// value range `(lo..hi)`. Both are matching mechanisms.
	if pe, ok := arg.(*syntax.ParenExpr); ok && pe != nil {
		if len(pe.List) > 1 {
			return "value list"
		}
		if len(pe.List) == 1 {
			if be, ok := pe.List[0].(*syntax.BinaryExpr); ok && be != nil && be.Op != nil && be.Op.Kind() == syntax.RANGE {
				return "value range"
			}
		}
		return ""
	}
	if _, ok := arg.(*syntax.PatternExpr); ok {
		return "pattern"
	}
	if _, ok := arg.(*syntax.LengthExpr); ok {
		return "length"
	}
	if ce, ok := arg.(*syntax.CallExpr); ok && ce != nil {
		if id, ok := ce.Fun.(*syntax.Ident); ok && id != nil {
			switch id.String() {
			case "permutation", "complement", "subset", "superset":
				return id.String()
			}
		}
	}
	return ""
}

func isOmitTok(e syntax.Expr) bool {
	v, ok := e.(*syntax.ValueLiteral)
	if !ok || v == nil || v.Tok == nil {
		return false
	}
	return v.Tok.Kind() == syntax.OMIT
}

func isAnyOrNoneTok(e syntax.Expr) bool {
	v, ok := e.(*syntax.ValueLiteral)
	if !ok || v == nil || v.Tok == nil {
		return false
	}
	return v.Tok.Kind() == syntax.MUL
}
