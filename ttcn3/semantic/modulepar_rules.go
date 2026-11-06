// modulepar_rules.go enforces ETSI ES 201 873-1 clause 5.4 (module
// parameters): the parameter must be a "value type" - port, timer,
// default, component, and template-restricted slots are forbidden.
//
// The rule is intentionally narrow: we only flag the kinds we can
// recognise syntactically. Composite types that *contain* a port
// (e.g. `record { port P p }`) are left to the runtime today; the
// suite's conformance tests live on the syntactic shapes.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkModuleparKinds(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	if mod == nil {
		return diags
	}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch v := d.Def.(type) {
		case *syntax.ModuleParameterGroup:
			for _, vd := range v.Decls {
				diags = append(diags, moduleparValueDeclDiags(vd)...)
			}
		case *syntax.ValueDecl:
			// `modulepar T x := ...;` lands here directly
			// (not wrapped in a ModuleParameterGroup). The
			// pre-existing path only catches the grouped
			// form `modulepar { T x; ... }`; without this
			// branch a single `modulepar default x` slips
			// through (NegSem_080201_ModuleParameters_003).
			if v.KindTok != nil && v.KindTok.Kind() == syntax.MODULEPAR {
				diags = append(diags, moduleparValueDeclDiags(v)...)
			}
		}
	}
	return diags
}

func moduleparValueDeclDiags(vd *syntax.ValueDecl) []Diagnostic {
	if vd == nil {
		return nil
	}
	var diags []Diagnostic
	// `modulepar template T x` is legal per ETSI 8.2.1:
	// "Module parameters are values or templates that may be
	// supplied by the test environment at runtime." We only
	// remember the restriction so the matcher-default check
	// below skips legitimate template defaults like `:= ?`.
	isTemplate := vd.TemplateRestriction != nil &&
		vd.TemplateRestriction.TemplateTok != nil
	if id, ok := vd.Type.(*syntax.Ident); ok && id.Tok != nil {
		switch id.Tok.Kind() {
		case syntax.TIMER:
			diags = append(diags, moduleparKindDiag(vd, "timer", "timer"))
		case syntax.PORT:
			diags = append(diags, moduleparKindDiag(vd, "port", "port"))
		}
		// `default` and `component` are surfaced as plain
		// idents; the parser keeps the literal name in
		// `id.Tok.String()`. We accept the common case of a
		// reserved-name match.
		switch id.String() {
		case "default":
			diags = append(diags, moduleparKindDiag(vd, "default", "default"))
		case "component":
			diags = append(diags, moduleparKindDiag(vd, "component", "component"))
		}
	}
	// A plain (non-template) modulepar's default value, if
	// provided, must be a value - not a matching mechanism like
	// `?`, `*`, complement, ifpresent, or a `value list`. ETSI
	// 8.2.1: "If no template modifier is present, the
	// TemplateBody shall resolve to a value."
	if !isTemplate {
		for _, d := range vd.Decls {
			if d == nil || d.Value == nil {
				continue
			}
			if what := valueMatcherViolation(d.Value); what != "" {
				diags = append(diags, Diagnostic{
					Code:     "modulepar-value-matcher-default",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"module parameter %q cannot have %s as its default; expected a value (ETSI 8.2.1)",
						declName(d), what),
					Node: d.Value,
					Span: syntax.SpanOf(d.Value),
				})
			}
		}
	}
	return diags
}

func moduleparKindDiag(vd *syntax.ValueDecl, kind, what string) Diagnostic {
	name := ""
	if len(vd.Decls) > 0 && vd.Decls[0] != nil && vd.Decls[0].Name != nil {
		name = vd.Decls[0].Name.String()
	}
	return Diagnostic{
		Code:     "modulepar-forbidden-kind",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"module parameter %q cannot be of kind %q (ETSI 5.4)",
			name, what),
		Node: vd,
		Span: syntax.SpanOf(vd),
	}
}
