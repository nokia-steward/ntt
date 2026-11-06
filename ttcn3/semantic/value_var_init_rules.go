// value_var_init_rules.go enforces ETSI ES 201 873-1 clause 11.1
// restriction d: a plain (non-template) variable initializer
// shall be a value expression. Matching mechanisms - `?`, `*`,
// `pattern`, value ranges, lists - are template constructs and
// must not appear in a `var T name := ...` declaration.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkValueVarInitRules(mod *syntax.Module) []Diagnostic {
	valueVars := collectValueVarNames(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			if v == nil || v.KindTok == nil ||
				v.KindTok.Kind() != syntax.VAR {
				return true
			}
			// `var template T x` is a template variable -
			// matchers are legitimate there.
			if v.TemplateRestriction != nil &&
				v.TemplateRestriction.TemplateTok != nil &&
				v.TemplateRestriction.TemplateTok.Kind() != syntax.ILLEGAL {
				return true
			}
			for _, d := range v.Decls {
				if d == nil || d.Value == nil {
					continue
				}
				if what := valueMatcherViolation(d.Value); what != "" {
					diags = append(diags, Diagnostic{
						Code:     "value-var-template-init",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"value variable %q cannot be initialised with %s (ETSI 11.1)",
							declName(d), what),
						Node: d.Value,
						Span: syntax.SpanOf(d.Value),
					})
				}
			}
		case *syntax.BinaryExpr:
			if v == nil || v.Op == nil || v.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhsId, ok := v.X.(*syntax.Ident)
			if !ok || lhsId == nil {
				return true
			}
			if !valueVars[lhsId.String()] {
				return true
			}
			if what := valueMatcherViolation(v.Y); what != "" {
				diags = append(diags, Diagnostic{
					Code:     "value-var-template-assign",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"value variable %q cannot be assigned %s (ETSI 11.1)",
						lhsId.String(), what),
					Node: v.Y,
					Span: syntax.SpanOf(v.Y),
				})
			}
		}
		return true
	})
	return diags
}

// collectValueVarNames builds the set of identifiers that name a
// `var T x` declaration (i.e. not `var template T x`). The
// assignment check uses this set to decide whether the LHS of
// `x := ...` is a value variable subject to ETSI 11.1.d.
func collectValueVarNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil &&
			vd.TemplateRestriction.TemplateTok.Kind() != syntax.ILLEGAL {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			out[d.Name.String()] = true
		}
		return true
	})
	return out
}

func declName(d *syntax.Declarator) string {
	if d == nil || d.Name == nil {
		return ""
	}
	return d.Name.String()
}

// valueMatcherViolation classifies expr against the matcher-only
// constructs of ETSI 11.1.d. Returns a human-readable label when
// expr is a top-level matcher; "" otherwise. We deliberately
// inspect only the syntactic shape of expr - not its subtrees -
// so a function call like `char(0,0,40,20)` whose Args happen to
// be a multi-element ParenExpr isn't mistaken for a value-list
// matcher. Matchers nested deep inside a value expression are out
// of scope; clause 11.1 only forbids matcher-shaped initialisers.
func valueMatcherViolation(expr syntax.Expr) string {
	switch x := expr.(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil {
			return ""
		}
		switch x.Tok.Kind() {
		case syntax.ANY:
			return "the AnyValue matcher `?`"
		case syntax.MUL:
			return "the AnyValueOrNone matcher `*`"
		}
	case *syntax.LengthExpr:
		return "a `length` restriction"
	case *syntax.PatternExpr:
		return "a `pattern` matcher"
	case *syntax.BinaryExpr:
		if x.Op != nil && x.Op.Kind() == syntax.RANGE {
			return "a value range matcher"
		}
	case *syntax.CallExpr:
		id, ok := x.Fun.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			return ""
		}
		switch id.String() {
		case "permutation", "complement", "subset", "superset":
			return "the `" + id.String() + "` matcher"
		}
	case *syntax.DecmatchExpr:
		return "a `decmatch` matcher"
	case *syntax.UnaryExpr:
		if x.Op != nil && x.Op.Kind() == syntax.IFPRESENT {
			return "the `ifpresent` matcher"
		}
	}
	return ""
}
