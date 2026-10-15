// template_reassign_rules.go enforces ETSI ES 201 873-1 clause
// 15.3: a global or local TemplateDecl is read-only after
// declaration. Reassigning it with `t := ...` is forbidden.
//
// Template-typed variables (`var template T t := ...`) remain
// writable; the check uses the TemplateDecl AST node to
// distinguish the two cases.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateReassignRules(mod *syntax.Module) []Diagnostic {
	globals := collectTemplateDeclNames(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		locals := collectLocalTemplateDeclNames(fn.Body)
		diags = append(diags, templateReassignDiags(fn.Body, globals, locals)...)
	}
	return diags
}

// collectTemplateDeclNames returns the set of TemplateDecl names
// declared at the module top level. These are immutable.
func collectTemplateDeclNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		td, ok := d.Def.(*syntax.TemplateDecl)
		if !ok || td.Name == nil {
			continue
		}
		out[td.Name.String()] = true
	}
	return out
}

// collectLocalTemplateDeclNames walks a function body for inline
// `template T name := ...` declarations (TemplateDecl nodes) and
// returns their names. We deliberately ignore the
// `var template T name := ...` form because those are mutable.
func collectLocalTemplateDeclNames(body *syntax.BlockStmt) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		out[td.Name.String()] = true
		return true
	})
	return out
}

func templateReassignDiags(
	body *syntax.BlockStmt,
	globals, locals map[string]bool,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		name := id.String()
		scope := ""
		switch {
		case locals[name]:
			scope = "local"
		case globals[name]:
			scope = "global"
		default:
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "template-decl-reassign",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s template %q is immutable after declaration (ETSI 15.3)",
				scope, name),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}
