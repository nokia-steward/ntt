// return_template_value_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 16.1: a function declared to
// return a value type T (no \`template\` restriction on the
// return spec) cannot directly \`return p\` where \`p\` is a
// formal parameter declared as a \`template ...\`.
//
// Example that is rejected:
//
//	function f_test(template octetstring p_ostring)
//	    return octetstring {
//	    return p_ostring;       // ← error
//	}
//
// We only flag the cleanest shape - the return expression
// must be a bare identifier matching a template-typed
// parameter. Compound expressions, valueof(), etc. are
// untouched: they may still be invalid, but proving so
// requires evaluation/inference outside this rule's scope.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkReturnTemplateValueRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.Return == nil {
			continue
		}
		if fn.Return.Restriction != nil {
			continue
		}
		templParams := templateParamNames(fn)
		if len(templParams) == 0 {
			continue
		}
		fname := ""
		if fn.Name != nil {
			fname = fn.Name.String()
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			rs, ok := n.(*syntax.ReturnStmt)
			if !ok || rs == nil || rs.Result == nil {
				return true
			}
			id, ok := rs.Result.(*syntax.Ident)
			if !ok || id == nil {
				return true
			}
			if !templParams[id.String()] {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "return-template-as-value",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"function %q has a value return type but returns template-typed parameter %q (ETSI 16.1)",
					fname, id.String()),
				Node: rs,
				Span: syntax.SpanOf(rs),
			})
			return true
		})
	}
	return diags
}

func templateParamNames(fn *syntax.FuncDecl) map[string]bool {
	out := map[string]bool{}
	if fn == nil || fn.Params == nil {
		return out
	}
	for _, fp := range fn.Params.List {
		if fp == nil || fp.Name == nil || fp.TemplateRestriction == nil {
			continue
		}
		if fp.TemplateRestriction.TemplateTok == nil {
			continue
		}
		out[fp.Name.String()] = true
	}
	return out
}
