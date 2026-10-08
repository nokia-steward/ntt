// signature_template_rules.go enforces the matching-mechanism
// restrictions on signature-shaped templates passed to the
// procedure-based send operations (`call`, `reply`, `raise`)
// in ETSI ES 201 873-1 clauses 22.3.1, 22.3.3 and 22.3.5.
//
// In short:
//
//   - `call(Sig:{...})`  -> all `in` and `inout` parameter slots
//     must hold a specific value (no `?`, `*`, `-`, `omit`).
//   - `reply(Sig:{...}) [value v]` -> all `out` and `inout`
//     parameter slots, plus the return value, must be specific.
//   - `raise(Sig, expr)` -> the exception value must be specific
//     (handled in port_ops.go's wildcard rule, not here).
//
// We rely on the syntactic shape of the actual: either the inline form
// `Sig:{...}` or a reference to a `template Sig t := {...}` declaration (the
// `Sig:tmplRef` COLON form resolves the named template's body too, so a
// call-safe template with `out := -` used in a reply is caught — ETSI 15.3
// restriction e, NegSem_1503_GlobalAndLocalTemplates_007). Anything outside
// those shapes (e.g. a formal parameter passed in from a caller) is left
// alone - the rule trades completeness for zero false positives.
//
// NOTE: the mirror `out := <value>` used in a `call` (the 15.3 case
// NegSem_..._008) is deliberately NOT flagged: the suite contradicts itself
// there (Sem_220304_getreply_operation_006 uses the same `call(S:{out:=v})`
// construct and expects accept), so a static rule cannot reject one without
// wrongly rejecting the other.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSignatureTemplateRules(mod *syntax.Module) []Diagnostic {
	sigParams := collectSignatureParamDirs(mod)
	if len(sigParams) == 0 {
		return nil
	}
	tmplTypes := collectTemplateTypes(mod)
	tmplBodies := collectAllTemplateBodies(mod)

	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
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
		op := opIdent.String()
		if op != "call" && op != "reply" {
			return true
		}
		if ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		first := ce.Args.List[0]
		sigName, body := resolveSignatureTemplate(first, tmplTypes, tmplBodies)
		if sigName == "" {
			return true
		}
		params, ok := sigParams[sigName]
		if !ok || len(params) == 0 {
			return true
		}
		// Matching-mechanism restriction (22.3): the slots that must be
		// SPECIFIC — `call` -> in/inout, `reply` -> out/inout. (Resolving
		// the named-template body above lets this also catch a call-safe
		// template — out := `-` — used in a reply: ETSI 15.3 restriction e.)
		var forbid map[syntax.Kind]bool
		switch op {
		case "call":
			forbid = map[syntax.Kind]bool{syntax.IN: true, syntax.INOUT: true}
		case "reply":
			forbid = map[syntax.Kind]bool{syntax.OUT: true, syntax.INOUT: true}
		}
		diags = append(diags, sigTemplateBodyDiags(body, params, forbid, op, sigName, tmplBodies)...)
		return true
	})
	return diags
}

// sigParamInfo captures the bits of a signature formal parameter
// we need to validate signature-template fields against. We only
// need the slot name and direction; types come from a parallel
// system we don't model yet.
type sigParamInfo struct {
	Name string
	Dir  syntax.Kind
}

// collectSignatureParamDirs returns, for every signature declared
// in the module, the ordered list of (param name, direction).
// Parameters without an explicit direction default to IN, which
// matches the ETSI grammar default for signature parameters.
func collectSignatureParamDirs(mod *syntax.Module) map[string][]sigParamInfo {
	out := map[string][]sigParamInfo{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd.Name == nil {
			continue
		}
		var params []sigParamInfo
		if sd.Params != nil {
			for _, fp := range sd.Params.List {
				if fp == nil || fp.Name == nil {
					continue
				}
				dir := syntax.IN
				if fp.Direction != nil {
					dir = fp.Direction.Kind()
				}
				params = append(params, sigParamInfo{
					Name: fp.Name.String(),
					Dir:  dir,
				})
			}
		}
		out[sd.Name.String()] = params
	}
	return out
}

// collectAllTemplateBodies behaves like collectTemplateBodies
// but also reaches into local `var template T x := ...`
// declarations inside function bodies. We don't merge the two
// helpers because the send-no-wildcard rule has been calibrated
// against a strictly module-scope view and broadening it would
// risk false positives on locally-defined relay templates.
func collectAllTemplateBodies(mod *syntax.Module) map[string]syntax.Expr {
	out := collectTemplateBodies(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.TemplateRestriction == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			out[dec.Name.String()] = dec.Value
		}
		return true
	})
	return out
}

// collectTemplateTypes returns a map from template name to the
// (bare) type ident that template binds. Used to resolve a
// `port.call(t)` reference where t is `template Sig t := {...}`.
func collectTemplateTypes(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		td, ok := d.Def.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil || td.Type == nil {
			continue
		}
		if id, ok := td.Type.(*syntax.Ident); ok && id != nil && id.Tok != nil {
			out[td.Name.String()] = id.String()
		}
	}
	return out
}

// resolveSignatureTemplate looks at the first actual argument
// of a `call(...)` / `reply(...)` op and returns the signature
// name + the composite body the user wrote, if both can be
// recovered statically. Returns ("", nil) for any shape we
// don't recognise.
//
// Two shapes are recognised:
//
//	Sig:{ p_a := ... }              (inline)
//	tmpl_ref                         where tmpl_ref is the name of
//	                                 `template Sig tmpl_ref := {...}`
func resolveSignatureTemplate(actual syntax.Expr, tmplTypes map[string]string, tmplBodies map[string]syntax.Expr) (string, syntax.Expr) {
	if pe, ok := actual.(*syntax.ParenExpr); ok && len(pe.List) == 1 {
		return resolveSignatureTemplate(pe.List[0], tmplTypes, tmplBodies)
	}
	if be, ok := actual.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.COLON {
		if id, ok := be.X.(*syntax.Ident); ok && id != nil && id.Tok != nil {
			body := be.Y
			// `Sig:tmplRef` — resolve a named template reference to its
			// composite body so the field-level rules see the actuals.
			if ref, ok := be.Y.(*syntax.Ident); ok && ref != nil && ref.Tok != nil {
				if b, has := tmplBodies[ref.String()]; has {
					body = b
				}
			}
			return id.String(), body
		}
		return "", nil
	}
	if id, ok := actual.(*syntax.Ident); ok && id != nil && id.Tok != nil {
		name := id.String()
		if sig, ok := tmplTypes[name]; ok {
			return sig, tmplBodies[name]
		}
	}
	return "", nil
}

// sigTemplateBodyDiags walks the composite literal that supplies
// the actuals for one signature template, matches each field
// against the corresponding signature parameter, and emits a
// diagnostic for every matching-mechanism token that lands in a
// forbidden direction slot.
//
// Two field shapes are supported:
//
//	{ p_par := ?, p_other := - }   (named assignments)
//	{ ?, -, 42 }                   (positional values)
//
// In the positional case we line up by index against the signature
// param list. Anything we can't pair (extra fields, out-of-order
// names) is silently skipped so the rule never falsely accuses a
// well-formed program.
func sigTemplateBodyDiags(body syntax.Expr, params []sigParamInfo, forbid map[syntax.Kind]bool, op, sigName string, tmplBodies map[string]syntax.Expr) []Diagnostic {
	if body == nil {
		return nil
	}
	cl, ok := body.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return nil
	}
	paramByName := map[string]sigParamInfo{}
	for _, p := range params {
		paramByName[p.Name] = p
	}

	var diags []Diagnostic
	for i, item := range cl.List {
		var p sigParamInfo
		var have bool
		if be, ok := item.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			name := identName(be.X)
			if name == "" {
				continue
			}
			p, have = paramByName[name]
			if !have {
				continue
			}
			if !forbid[p.Dir] {
				continue
			}
			if mech := deepMatchingMechanism(be.Y, tmplBodies); mech != "" {
				diags = append(diags, signatureTemplateDiag(op, sigName, p, mech, be.Y))
			}
		} else {
			if i >= len(params) {
				continue
			}
			p = params[i]
			if !forbid[p.Dir] {
				continue
			}
			if mech := deepMatchingMechanism(item, tmplBodies); mech != "" {
				diags = append(diags, signatureTemplateDiag(op, sigName, p, mech, item))
			}
		}
	}
	return diags
}

// deepMatchingMechanism returns the first matching-mechanism
// token reachable from expr (looking inside composite literals
// and following bare template references through tmplBodies),
// or "" when the expression is entirely specific. We use this
// instead of a flat matchingMechanismName so a signature-field
// value like `{ field1 := 0, field2 := ? }` is rejected too.
//
// A visited set guards against template-ref cycles.
func deepMatchingMechanism(expr syntax.Expr, tmplBodies map[string]syntax.Expr) string {
	visited := map[string]bool{}
	var walk func(e syntax.Expr) string
	walk = func(e syntax.Expr) string {
		if e == nil {
			return ""
		}
		if mech := matchingMechanismName(e); mech != "" {
			return mech
		}
		switch v := e.(type) {
		case *syntax.CompositeLiteral:
			for _, c := range v.List {
				if be, ok := c.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
					if mech := walk(be.Y); mech != "" {
						return mech
					}
					continue
				}
				if mech := walk(c); mech != "" {
					return mech
				}
			}
		case *syntax.ParenExpr:
			for _, c := range v.List {
				if mech := walk(c); mech != "" {
					return mech
				}
			}
		case *syntax.Ident:
			name := v.String()
			if visited[name] {
				return ""
			}
			visited[name] = true
			if body, ok := tmplBodies[name]; ok {
				return walk(body)
			}
		}
		return ""
	}
	return walk(expr)
}

// matchingMechanismName returns the human-readable name of the
// matching mechanism if expr is a bare AnyValue / AnyOrNone /
// NoValue / Omit token, otherwise "". We do NOT recurse into
// composite shapes because the spec only forbids matching at
// the top of the parameter slot; field-level matching inside a
// nested record/set is fine.
func matchingMechanismName(expr syntax.Expr) string {
	if expr == nil {
		return ""
	}
	if pe, ok := expr.(*syntax.ParenExpr); ok && len(pe.List) == 1 {
		return matchingMechanismName(pe.List[0])
	}
	if vl, ok := expr.(*syntax.ValueLiteral); ok && vl != nil && vl.Tok != nil {
		switch vl.Tok.Kind() {
		case syntax.ANY:
			return "?"
		case syntax.MUL:
			return "*"
		case syntax.SUB:
			return "-"
		case syntax.OMIT:
			return "omit"
		}
	}
	if id, ok := expr.(*syntax.Ident); ok && id != nil && id.Tok != nil {
		if id.Tok.Kind() == syntax.OMIT {
			return "omit"
		}
	}
	return ""
}

func signatureTemplateDiag(op, sigName string, p sigParamInfo, mech string, node syntax.Node) Diagnostic {
	return Diagnostic{
		Code:     "signature-template-matching-not-allowed",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"%s on signature %q: parameter %q is %s; matching mechanism `%s` is not allowed in this slot (ETSI 22.3)",
			op, sigName, p.Name, dirName(p.Dir), mech),
		Node: node,
		Span: syntax.SpanOf(node),
	}
}

func dirName(k syntax.Kind) string {
	switch k {
	case syntax.IN:
		return "in"
	case syntax.OUT:
		return "out"
	case syntax.INOUT:
		return "inout"
	}
	return "in"
}
