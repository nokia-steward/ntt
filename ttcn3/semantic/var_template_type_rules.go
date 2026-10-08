// var_template_type_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 11.2: a template variable must
// be assigned a value of a compatible type.
//
// We only handle the cleanest shape:
//   - the declarator is "var template <basic-type> name := <Ident>"
//   - the right-hand side is a bare identifier that resolves
//     to a sibling local "var <basic-type> name := ..." declared
//     earlier in the same function body
//
// If the two basic types differ (e.g. integer→float), we emit
// a static error. Anything more elaborate (composite, sub-type,
// component members, parameters) is left to richer typing logic.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkVarTemplateTypeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		checkVarTemplateTypesInBlock(fn.Body, map[string]string{}, &diags)
	}
	return diags
}

func checkVarTemplateTypesInBlock(
	body *syntax.BlockStmt,
	scope map[string]string,
	diags *[]Diagnostic,
) {
	if body == nil {
		return
	}
	for _, st := range body.Stmts {
		ds, ok := st.(*syntax.DeclStmt)
		if !ok || ds == nil {
			if bs, ok := st.(*syntax.BlockStmt); ok && bs != nil {
				child := map[string]string{}
				for k, v := range scope {
					child[k] = v
				}
				checkVarTemplateTypesInBlock(bs, child, diags)
			}
			continue
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			continue
		}
		if vd.KindTok.Kind() != syntax.VAR {
			continue
		}
		want := basicTypeNameOf(vd.Type)
		isTemplate := vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil {
				continue
			}
			name := dc.Name.String()
			if dc.Value == nil {
				if !isTemplate && want != "" {
					scope[name] = want
				}
				continue
			}
			if isTemplate && want != "" {
				if id, ok := dc.Value.(*syntax.Ident); ok && id != nil {
					if got, ok := scope[id.String()]; ok && got != "" && got != want {
						*diags = append(*diags, Diagnostic{
							Code:     "var-template-init-type-mismatch",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"template variable %q has type %q but its initialiser %q has type %q (ETSI 11.2)",
								name, want, id.String(), got),
							Node: dc,
							Span: syntax.SpanOf(dc),
						})
					}
				}
			}
			if !isTemplate && want != "" {
				scope[name] = want
			}
		}
	}
}
