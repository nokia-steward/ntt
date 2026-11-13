// altstep_decl_ordering_rules.go enforces ETSI ES 201 873-1
// clause 16.2: in an altstep body, all declarations
// (var / const / timer) must precede the first alternative
// branch (CommClause). A declaration placed after the alt
// branches is not allowed by the grammar of altstep bodies.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAltstepDeclOrderingRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.KindTok == nil {
			continue
		}
		if fn.KindTok.Kind() != syntax.ALTSTEP {
			continue
		}
		sawAltBranch := false
		for _, st := range fn.Body.Stmts {
			if _, ok := st.(*syntax.CommClause); ok {
				sawAltBranch = true
				continue
			}
			if !sawAltBranch {
				continue
			}
			ds, ok := st.(*syntax.DeclStmt)
			if !ok || ds == nil {
				continue
			}
			name := ""
			if vd, ok := ds.Decl.(*syntax.ValueDecl); ok && vd != nil {
				for _, dc := range vd.Decls {
					if dc != nil && dc.Name != nil {
						name = dc.Name.String()
						break
					}
				}
			}
			diags = append(diags, Diagnostic{
				Code:     "altstep-late-declaration",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"declaration of %q appears after an alt branch; all declarations in an altstep body must precede the alternatives (ETSI 16.2)",
					name),
				Node: ds,
				Span: syntax.SpanOf(ds),
			})
		}
	}
	return diags
}
