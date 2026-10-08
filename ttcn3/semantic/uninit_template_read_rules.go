// uninit_template_read_rules.go enforces ETSI ES 201 873-1
// clause 11.2: a `var template T x;` declaration that has
// not been assigned to may not be read by a sibling
// declaration's initializer expression.
//
// We deliberately keep the scope tighter than the general
// uninit_read_rules.go check:
//
//   - both the producer (uninitialised declarator) and the
//     consumer (initializer that reads it) must carry a
//     `template` restriction. Plain `var T` declarations are
//     handled by the broader rule, which intentionally skips
//     bare-RHS reads to avoid out/inout false positives;
//   - the read must be a bare identifier directly on the RHS
//     of a `:=` initializer; nested expressions are deferred
//     to the broader rule;
//   - the analysis is strictly per-BlockStmt straight line;
//     any control-flow statement resets the state.
//
// These narrow conditions are enough to reject
// NegSem_1102_TemplateVars_001 without touching Sem
// positives that legitimately copy from a freshly-assigned
// template variable.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkUninitTemplateReadRules(mod *syntax.Module) []Diagnostic {
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
		diags = append(diags, checkUninitTemplateInBlock(fn.Body)...)
	}
	return diags
}

func checkUninitTemplateInBlock(body *syntax.BlockStmt) []Diagnostic {
	if body == nil {
		return nil
	}
	var diags []Diagnostic
	uninit := map[string]*syntax.Declarator{}
	for _, stmt := range body.Stmts {
		ds, ok := stmt.(*syntax.DeclStmt)
		if !ok {
			break
		}
		vd, ok := ds.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			break
		}
		if vd.TemplateRestriction == nil {
			continue
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil {
				continue
			}
			if dc.Value != nil {
				if id, ok := dc.Value.(*syntax.Ident); ok && id != nil {
					name := id.String()
					if _, bad := uninit[name]; bad {
						diags = append(diags, Diagnostic{
							Code:     "uninit-template-read",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"template variable %q is read before it is initialised (ETSI 11.2)",
								name),
							Node: dc.Value,
							Span: syntax.SpanOf(dc.Value),
						})
					}
				}
				delete(uninit, dc.Name.String())
				continue
			}
			uninit[dc.Name.String()] = dc
		}
	}
	return diags
}
