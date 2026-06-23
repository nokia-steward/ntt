// template_omit_assign_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 15.8: a template variable with
// the \`(omit)\` restriction can only hold a value or the
// \`omit\` symbol - it cannot hold a template that uses any
// matching mechanism (\`?\`, \`*\`, \`pattern\`, complement,
// ?length, etc.).
//
// Example that is rejected:
//
//	var template (omit) ExampleType v_omit;
//	template (present) ExampleType MyT := { a := ?, b := false };
//	v_omit := MyT;          // ← error, MyT contains \`?\`
//
// We only flag the cleanest shape:
//   - The LHS of an assignment is a bare Ident that
//     resolves to a template (omit) variable or component
//     member.
//   - The RHS is either an immediate composite literal /
//     matching expression, or a bare Ident that resolves to
//     a module-level template declaration. Two levels of
//     template indirection (T := A; A := { ... ? ... }) are
//     followed.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateOmitAssignRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	templates := collectModuleTemplateDecls(mod)
	omitVars := collectOmitTemplateVars(mod)
	if len(omitVars) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		es, ok := n.(*syntax.ExprStmt)
		if !ok || es == nil || es.Expr == nil {
			return true
		}
		be, ok := es.Expr.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		lhs, ok := be.X.(*syntax.Ident)
		if !ok || lhs == nil {
			return true
		}
		if !omitVars[lhs.String()] {
			return true
		}
		if reason, hasMatch := exprHasMatchingMechanism(be.Y, templates, map[string]bool{}); hasMatch {
			diags = append(diags, Diagnostic{
				Code:     "template-omit-assign-matching",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"template variable %q has restriction (omit); the assigned template value uses %s, which is not allowed (ETSI 15.8)",
					lhs.String(), reason),
				Node: es,
				Span: syntax.SpanOf(es),
			})
		}
		return true
	})
	return diags
}

func collectModuleTemplateDecls(mod *syntax.Module) map[string]*syntax.TemplateDecl {
	out := map[string]*syntax.TemplateDecl{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		td, ok := d.Def.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			continue
		}
		out[td.Name.String()] = td
	}
	return out
}

func collectOmitTemplateVars(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.TemplateRestriction == nil {
			return true
		}
		if vd.TemplateRestriction.Tok == nil {
			return true
		}
		if vd.TemplateRestriction.Tok.Kind() != syntax.OMIT {
			return true
		}
		for _, dc := range vd.Decls {
			if dc != nil && dc.Name != nil {
				out[dc.Name.String()] = true
			}
		}
		return true
	})
	return out
}

func exprHasMatchingMechanism(
	expr syntax.Expr,
	templates map[string]*syntax.TemplateDecl,
	seen map[string]bool,
) (string, bool) {
	if expr == nil {
		return "", false
	}
	switch e := expr.(type) {
	case *syntax.ValueLiteral:
		if e.Tok == nil {
			return "", false
		}
		switch e.Tok.Kind() {
		case syntax.ANY:
			return "the `?` matching symbol", true
		case syntax.MUL:
			return "the `*` matching symbol", true
		}
	case *syntax.PatternExpr:
		return "a `pattern` matcher", true
	case *syntax.LengthExpr:
		return "a `length` matching constraint", true
	case *syntax.UnaryExpr:
		if e.Op != nil && e.Op.Kind() == syntax.NOT {
			return "the `complement` matcher", true
		}
	case *syntax.Ident:
		if e == nil {
			return "", false
		}
		name := e.String()
		if seen[name] {
			return "", false
		}
		seen[name] = true
		td, ok := templates[name]
		if !ok || td == nil {
			return "", false
		}
		return exprHasMatchingMechanism(td.Value, templates, seen)
	case *syntax.CompositeLiteral:
		for _, elem := range e.List {
			if reason, ok := exprHasMatchingMechanism(elem, templates, seen); ok {
				return reason, true
			}
		}
	case *syntax.BinaryExpr:
		if e.Op != nil && e.Op.Kind() == syntax.ASSIGN {
			return exprHasMatchingMechanism(e.Y, templates, seen)
		}
		if e.Op != nil && e.Op.Kind() == syntax.RANGE {
			return "a range matcher", true
		}
	}
	return "", false
}
