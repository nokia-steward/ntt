// component_body_decl_rules.go enforces ETSI ES 201 873-1
// clause 8: a component body may only contain declarations
// (variables, constants, templates, timers, ports). Any other
// statement (expression, assignment, control flow, etc.) is
// not allowed.
//
// In particular the parser tolerantly accepts malformed
// declarations such as
//
//	type component C {
//	    timer t 0.0;   // missing ':='
//	}
//
// by treating \`0.0\` as a separate ExprStmt that follows the
// (otherwise valid) \`timer t\` declaration. The component
// body therefore ends up with a non-DeclStmt sibling, which
// this rule flags as \`component-body-non-decl\`.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkComponentBodyDeclRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ctd, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ctd == nil || ctd.Body == nil {
			continue
		}
		for _, st := range ctd.Body.Stmts {
			if st == nil {
				continue
			}
			if _, ok := st.(*syntax.DeclStmt); ok {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "component-body-non-decl",
				Severity: SeverityError,
				Message: "component body contains a non-declaration statement; only var/const/template/timer/port declarations are allowed (ETSI 8)",
				Node:     st,
				Span:     syntax.SpanOf(st),
			})
		}
	}
	return diags
}
