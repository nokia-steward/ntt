// value_template_kinds.go implements the static check from ETSI ES
// 201 873-1 clause 5.4.2: template values cannot be passed where a
// formal parameter expects a plain value, and vice-versa. The
// in/out/inout direction and the (im)mutability of the actual all
// factor in.
//
// The analyser builds a scope tree on demand:
//
//   - Pass 1 collects module-level template / const / modulepar /
//     function declarations.
//
//   - For each function-like declaration (`function`, `altstep`,
//     `testcase`, parameterised `template`) we run a scoped pass:
//     formals and local `var T x` declarations shadow module-level
//     bindings so that two functions can each have a parameter
//     called `p_val` without confusing the value / template kind
//     classification.
//
// This catches the NegSem_050402_actual_parameters_* family without
// needing a full type system. It will miss expressions that reuse
// templates indirectly (e.g. arithmetic on templates), but those are
// outside the simple "template -> value" pattern the spec illustrates.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// kindEnv is the per-scope classification of identifiers used by
// the cross-check loop below.
type kindEnv struct {
	templates      map[string]bool
	templateConsts map[string]bool
	valueVars      map[string]bool
	consts         map[string]bool
	funcs          map[string]*syntax.FormalPars
}

func newKindEnv() *kindEnv {
	return &kindEnv{
		templates:      map[string]bool{},
		templateConsts: map[string]bool{},
		valueVars:      map[string]bool{},
		consts:         map[string]bool{},
		funcs:          map[string]*syntax.FormalPars{},
	}
}

// extend returns a shallow child of env with name set to a single
// kind classification: when setTemplate is true, name is marked as a
// template binding (and the value-binding mark is cleared);
// otherwise it is marked as a value binding (and the template /
// template-const marks are cleared). Subsequent lookups in env are
// not affected; only the returned child.
func (env *kindEnv) clone() *kindEnv {
	out := newKindEnv()
	for k, v := range env.templates {
		out.templates[k] = v
	}
	for k, v := range env.templateConsts {
		out.templateConsts[k] = v
	}
	for k, v := range env.valueVars {
		out.valueVars[k] = v
	}
	for k, v := range env.consts {
		out.consts[k] = v
	}
	for k, v := range env.funcs {
		out.funcs[k] = v
	}
	return out
}

// shadowAsTemplate marks name as a template binding in env and
// removes any prior value-variable mark for the same name. Used
// when a local declaration shadows an outer binding.
func (env *kindEnv) shadowAsTemplate(name string) {
	if name == "" {
		return
	}
	env.templates[name] = true
	delete(env.valueVars, name)
}

// shadowAsValue is the mirror of shadowAsTemplate for plain value
// bindings.
func (env *kindEnv) shadowAsValue(name string) {
	if name == "" {
		return
	}
	env.valueVars[name] = true
	delete(env.templates, name)
	delete(env.templateConsts, name)
}

func (a *Analyzer) checkValueTemplateArgKinds(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	// Pass 1: build the module-level environment. Function formals
	// are NOT added here - they live in the per-function scope
	// below.
	modEnv := newKindEnv()
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		switch x := n.(type) {
		case *syntax.TemplateDecl:
			if x.Name != nil {
				name := syntax.Name(x.Name)
				modEnv.templates[name] = true
				modEnv.templateConsts[name] = true
				if x.Params != nil {
					modEnv.funcs[name] = x.Params
				}
			}
		case *syntax.ValueDecl:
			isTemplateTyped := x.TemplateRestriction != nil || hasTemplateKeyword(x.Type)
			isConstOrMpar := false
			if x.KindTok != nil {
				switch x.KindTok.Kind() {
				case syntax.CONST, syntax.MODULEPAR:
					isConstOrMpar = true
				}
			}
			for _, dec := range x.Decls {
				if dec == nil || dec.Name == nil || dec.Name.Tok == nil {
					continue
				}
				name := dec.Name.String()
				if isTemplateTyped {
					modEnv.templates[name] = true
					if isConstOrMpar {
						modEnv.templateConsts[name] = true
					}
				} else {
					modEnv.valueVars[name] = true
				}
				if isConstOrMpar {
					modEnv.consts[name] = true
				}
			}
		case *syntax.FuncDecl:
			if x.Name != nil {
				modEnv.funcs[syntax.Name(x.Name)] = x.Params
			}
		}
		return true
	})

	// Pass 2: walk each function-like body in its own scope.
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		var (
			body   *syntax.BlockStmt
			params *syntax.FormalPars
		)
		switch x := d.Def.(type) {
		case *syntax.FuncDecl:
			body = x.Body
			params = x.Params
		case *syntax.TemplateDecl:
			// Parameterised templates can contain CallExprs in
			// their initialisers, but those are evaluated as
			// part of the template definition. We currently
			// don't classify field initialisers; the body
			// walk below handles them only if they appear in
			// a Block-like context, which TemplateDecl does
			// not currently expose. Skip.
			continue
		default:
			continue
		}
		if body == nil {
			continue
		}
		scope := modEnv.clone()
		if params != nil {
			for _, p := range params.List {
				if p == nil || p.Name == nil {
					continue
				}
				name := syntax.Name(p.Name)
				if p.TemplateRestriction != nil || hasTemplateKeyword(p.Type) {
					scope.shadowAsTemplate(name)
				} else {
					scope.shadowAsValue(name)
				}
			}
		}
		collectLocalDecls(body, scope)
		diags = append(diags, checkCallsInBody(body, scope)...)
	}

	// Module control part is one of mod.Defs's ControlPart entries
	// - covered below via a second pass that picks them up by type.
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		cp, ok := d.Def.(*syntax.ControlPart)
		if !ok || cp == nil || cp.Body == nil {
			continue
		}
		scope := modEnv.clone()
		collectLocalDecls(cp.Body, scope)
		diags = append(diags, checkCallsInBody(cp.Body, scope)...)
	}

	return diags
}

// collectLocalDecls walks body and adds every `var T x` / `var
// template T x` declaration it sees to env, shadowing any outer
// binding of the same name.
func collectLocalDecls(body syntax.Node, env *kindEnv) {
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		vd, ok := n.(*syntax.ValueDecl)
		if !ok {
			return true
		}
		isTemplate := vd.TemplateRestriction != nil || hasTemplateKeyword(vd.Type)
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Name.Tok == nil {
				continue
			}
			name := dec.Name.String()
			if isTemplate {
				env.shadowAsTemplate(name)
			} else {
				env.shadowAsValue(name)
			}
		}
		return true
	})
}

// checkCallsInBody runs the per-CallExpr cross-checks in body using
// the supplied environment. The environment is read-only - callers
// build it before invoking this helper.
func checkCallsInBody(body syntax.Node, env *kindEnv) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok {
			return true
		}
		params, ok := env.funcs[id.String()]
		if !ok || params == nil || ce.Args == nil {
			return true
		}
		diags = append(diags, checkOneCall(ce, id, params, env)...)
		return true
	})
	return diags
}

// checkOneCall is the cross-check kernel. All the per-call rules
// (arity, named-arg ordering, lvalue-ness, template/value kind
// matching) live here.
func checkOneCall(ce *syntax.CallExpr, id *syntax.Ident, params *syntax.FormalPars, env *kindEnv) []Diagnostic {
	var diags []Diagnostic
	formals := params.List
	actuals := ce.Args.List

	// Arity: too many actuals.
	if len(actuals) > len(formals) {
		diags = append(diags, Diagnostic{
			Code:     "too-many-actual-parameters",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"call to %q has %d actuals but %q has only %d formals",
				id.String(), len(actuals), id.String(), len(formals)),
			Node: ce,
			Span: syntax.SpanOf(ce),
		})
	}

	// Mixed-notation: positional after a named arg.
	seenNamed := false
	for _, a := range actuals {
		isNamed := false
		if be, ok := a.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			isNamed = true
		}
		if seenNamed && !isNamed {
			diags = append(diags, Diagnostic{
				Code:     "positional-after-named-arg",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"positional actual after named assignment in call to %q",
					id.String()),
				Node: a,
				Span: syntax.SpanOf(a),
			})
		}
		if isNamed {
			seenNamed = true
		}
	}

	// Arity: too few required actuals (positional form only).
	if !seenNamed {
		required := 0
		for i, f := range formals {
			if f == nil {
				continue
			}
			dir := ""
			if f.Direction != nil {
				dir = f.Direction.String()
			}
			if f.Value == nil && dir != "out" {
				required = i + 1
			}
		}
		if len(actuals) < required {
			diags = append(diags, Diagnostic{
				Code:     "too-few-actual-parameters",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"call to %q has %d actuals but %q requires %d (no defaults)",
					id.String(), len(actuals), id.String(), required),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
		}
	}

	formalByName := map[string]*syntax.FormalPar{}
	for _, f := range formals {
		if f != nil && f.Name != nil {
			formalByName[syntax.Name(f.Name)] = f
		}
	}
	for i, rawA := range actuals {
		var formal *syntax.FormalPar
		a := rawA
		// Named-argument shape: `name := value` overrides
		// the positional index. The named formal is
		// resolved by spelling so we don't misalign the
		// rest of the call.
		if be, ok := a.(*syntax.BinaryExpr); ok && be.Op != nil &&
			be.Op.Kind() == syntax.ASSIGN {
			if id, ok := be.X.(*syntax.Ident); ok {
				formal = formalByName[id.String()]
			}
			a = be.Y
		}
		if formal == nil {
			if i >= len(formals) {
				break
			}
			formal = formals[i]
		}
		if formal == nil {
			continue
		}
		formalDir := ""
		if formal.Direction != nil {
			formalDir = formal.Direction.String()
		}
		if isDashDefault(a) {
			if formalDir != "out" && formalDir != "inout" {
				if formal.Value == nil {
					diags = append(diags, Diagnostic{
						Code:     "dash-without-default",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"dash placeholder for %q has no default in call to %q",
							syntax.Name(formal.Name), id.String()),
						Node: a,
						Span: syntax.SpanOf(a),
					})
				}
			}
			continue
		}
		formalTemplate := formal.TemplateRestriction != nil || hasTemplateKeyword(formal.Type)
		formalName := ""
		if formal.Name != nil {
			formalName = syntax.Name(formal.Name)
		}

		// Rule A: template-typed actual into a value-typed `in`.
		// The actual can be a bare identifier OR a selector /
		// index path rooted at a template variable - both are
		// equally invalid because the leaf expression carries
		// the template restriction of the root.
		if formalDir != "out" && formalDir != "inout" && !formalTemplate {
			if root := rootLValueIdent(a); root != "" && env.templates[root] && !env.valueVars[root] {
				diags = append(diags, Diagnostic{
					Code:     "template-to-value-arg",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"cannot pass template %q to value formal parameter %q of %q",
						root, formalName, id.String()),
					Node: a,
					Span: syntax.SpanOf(a),
				})
			}
		}

		// Rule B: out / inout require a writable lvalue.
		if formalDir == "out" || formalDir == "inout" {
			if isLiteralExpr(a) {
				diags = append(diags, Diagnostic{
					Code:     "non-lvalue-out-inout-arg",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"literal cannot be passed to %s formal parameter %q of %q",
						formalDir, formalName, id.String()),
					Node: a,
					Span: syntax.SpanOf(a),
				})
			} else if act, ok := a.(*syntax.Ident); ok && env.consts[act.String()] {
				diags = append(diags, Diagnostic{
					Code:     "non-lvalue-out-inout-arg",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"const/modulepar %q cannot be passed to %s formal parameter %q of %q",
						act.String(), formalDir, formalName, id.String()),
					Node: a,
					Span: syntax.SpanOf(a),
				})
			} else if !isLValueExpr(a) {
				diags = append(diags, Diagnostic{
					Code:     "non-lvalue-out-inout-arg",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"non-lvalue expression cannot be passed to %s formal parameter %q of %q",
						formalDir, formalName, id.String()),
					Node: a,
					Span: syntax.SpanOf(a),
				})
			}
			// inout with a value formal: template actual is rejected.
			// As for Rule A, accept selector / index paths into a
			// template variable so `v_tmpl.field`/`v_tmpl[0]` is
			// caught - the leaf inherits the root's template
			// nature.
			if !formalTemplate && formalDir == "inout" {
				if root := rootLValueIdent(a); root != "" && env.templates[root] && !env.valueVars[root] {
					diags = append(diags, Diagnostic{
						Code:     "template-to-value-arg",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"cannot pass template %q to inout value formal parameter %q of %q",
							root, formalName, id.String()),
						Node: a,
						Span: syntax.SpanOf(a),
					})
				}
			}
			// Template constant -> out / inout: forbidden.
			if act, ok := a.(*syntax.Ident); ok && env.templateConsts[act.String()] {
				diags = append(diags, Diagnostic{
					Code:     "non-lvalue-out-inout-arg",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"template constant %q cannot be passed to %s formal parameter %q of %q",
						act.String(), formalDir, formalName, id.String()),
					Node: a,
					Span: syntax.SpanOf(a),
				})
			}
			// Template-typed out / inout: value variable / value
			// formal parameter actual is forbidden.
			if formalTemplate {
				if root := rootLValueIdent(a); root != "" {
					if env.valueVars[root] && !env.templates[root] {
						diags = append(diags, Diagnostic{
							Code:     "value-to-out-template-arg",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"value variable %q cannot be passed to %s template formal parameter %q of %q",
								root, formalDir, formalName, id.String()),
							Node: a,
							Span: syntax.SpanOf(a),
						})
					}
				}
			}
		}
	}
	return diags
}

// rootLValueIdent returns the base identifier name of a chain like
// `x.f[2].g`, or "" if expr is not an lvalue chain rooted at an
// identifier.
func rootLValueIdent(expr syntax.Expr) string {
	for {
		switch e := expr.(type) {
		case *syntax.Ident:
			if e.Tok == nil {
				return ""
			}
			return e.String()
		case *syntax.SelectorExpr:
			expr = e.X
			continue
		case *syntax.IndexExpr:
			expr = e.X
			continue
		case *syntax.ParenExpr:
			if len(e.List) == 1 {
				expr = e.List[0]
				continue
			}
			return ""
		default:
			return ""
		}
	}
}

// isLValueExpr reports whether expr is a syntactically valid lvalue
// - an identifier, an indexed access into one (`x[i]`), or a field
// access (`x.f`). CallExpr, BinaryExpr, ValueLiteral and others are
// not lvalues and cannot be passed to out/inout parameters.
func isLValueExpr(expr syntax.Expr) bool {
	for {
		switch e := expr.(type) {
		case *syntax.Ident:
			return true
		case *syntax.SelectorExpr:
			expr = e.X
			continue
		case *syntax.IndexExpr:
			expr = e.X
			continue
		case *syntax.ParenExpr:
			if len(e.List) == 1 {
				expr = e.List[0]
				continue
			}
			return false
		default:
			return false
		}
	}
}

// isLiteralExpr reports whether expr is a non-Ident literal value.
// We use this to flag literals being passed to out / inout formals.
func isLiteralExpr(expr syntax.Expr) bool {
	if expr == nil {
		return false
	}
	if v, ok := expr.(*syntax.ValueLiteral); ok && v.Tok != nil {
		// Dash default is a parser surface for `:= -` and is
		// handled by the template_restrictions pass.
		return v.Tok.Kind() != syntax.SUB
	}
	return false
}

// hasTemplateKeyword reports whether the type expression begins
// with the `template` keyword.
func hasTemplateKeyword(expr syntax.Expr) bool {
	if expr == nil {
		return false
	}
	var first syntax.Token
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if first != nil || n == nil {
			return false
		}
		if t, ok := n.(syntax.Token); ok && t != nil {
			first = t
			return false
		}
		return true
	})
	if first == nil {
		return false
	}
	return first.String() == "template"
}
