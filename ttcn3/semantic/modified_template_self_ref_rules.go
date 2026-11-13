// modified_template_self_ref_rules.go enforces ETSI ES 201 873-1
// clause 15.5: a `modifies` template shall not name itself as its
// base template. Self-referential modifications would form a
// cycle in the template-modification graph and are statically
// undefined.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkModifiedTemplateSelfRefRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	byName := map[string]*syntax.TemplateDecl{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		byName[td.Name.String()] = td
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil || td.Base == nil {
			return true
		}
		baseName := identName(td.Base)
		if baseName == "" {
			return true
		}
		if baseName == td.Name.String() {
			diags = append(diags, Diagnostic{
				Code:     "modified-template-self-reference",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"template %q cannot modify itself (ETSI 15.5)",
					td.Name.String()),
				Node: td,
				Span: syntax.SpanOf(td),
			})
			return true
		}
		base, ok := byName[baseName]
		if !ok || base == nil {
			return true
		}
		if reason := templateParamNamesMismatch(base.Params, td.Params); reason != "" {
			diags = append(diags, Diagnostic{
				Code:     "modified-template-param-mismatch",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"template %q modifies %q but %s (ETSI 15.5)",
					td.Name.String(), baseName, reason),
				Node: td,
				Span: syntax.SpanOf(td),
			})
		}
		if d := templateParamDashWithoutBase(base.Params, td.Params); d != nil {
			diags = append(diags, Diagnostic{
				Code:     "modified-template-dash-without-base-default",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"template %q modifies %q: parameter %q uses `-` default but the base parameter has no default (ETSI 15.5)",
					td.Name.String(), baseName, paramName(d)),
				Node: d,
				Span: syntax.SpanOf(d),
			})
		}
		return true
	})
	return diags
}

// templateParamDashWithoutBase scans the modified template's
// parameter list for a `name := -` default and returns the first
// offender whose corresponding base parameter has no default
// value to inherit. Returns nil when every dash default has a
// base default backing it.
func templateParamDashWithoutBase(base, mod *syntax.FormalPars) *syntax.FormalPar {
	if base == nil || mod == nil {
		return nil
	}
	if len(base.List) != len(mod.List) {
		return nil
	}
	for i, mp := range mod.List {
		if mp == nil || mp.Value == nil {
			continue
		}
		lit, ok := mp.Value.(*syntax.ValueLiteral)
		if !ok || lit == nil || lit.Tok == nil || lit.Tok.String() != "-" {
			continue
		}
		bp := base.List[i]
		if bp == nil || bp.Value == nil {
			return mp
		}
	}
	return nil
}

// templateParamNamesMismatch returns a non-empty reason when the
// modified template's parameter list is incompatible with the
// base template's.  ETSI 15.5 inherits the base parameters and
// allows the modifier to append new ones; the first len(base)
// slots must match by name (and by type when both sides give one).
// Strictly fewer parameters than the base is illegal.
func templateParamNamesMismatch(base, mod *syntax.FormalPars) string {
	var bList, mList []*syntax.FormalPar
	if base != nil {
		bList = base.List
	}
	if mod != nil {
		mList = mod.List
	}
	if len(mList) < len(bList) {
		return fmt.Sprintf(
			"the parameter list has %d entries while the base has %d",
			len(mList), len(bList))
	}
	for i := range bList {
		bn, mn := paramName(bList[i]), paramName(mList[i])
		if bn != mn {
			return fmt.Sprintf(
				"parameter %d is named %q but the base names it %q",
				i+1, mn, bn)
		}
		bt, mt := paramTypeName(bList[i]), paramTypeName(mList[i])
		if bt != "" && mt != "" && bt != mt {
			return fmt.Sprintf(
				"parameter %d %q has type %q but the base declares %q",
				i+1, mn, mt, bt)
		}
	}
	return ""
}

func paramTypeName(p *syntax.FormalPar) string {
	if p == nil || p.Type == nil {
		return ""
	}
	return identName(p.Type)
}

func paramName(p *syntax.FormalPar) string {
	if p == nil || p.Name == nil {
		return ""
	}
	return p.Name.String()
}
