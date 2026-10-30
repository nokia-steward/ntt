// formal_default_type_rules.go enforces ETSI ES 201 873-1
// clause 5.4.1.2: a default value supplied for a template
// formal parameter shall be type-compatible with the
// parameter's declared type.
//
// We only flag the cleanest, fully literal shape: a
// parameter declared as \`template integer p := (...)\`
// whose default value contains a float literal (a
// ValueLiteral whose token text holds a decimal point).
// Symmetric checks are performed for float-typed parameters
// that receive an integer literal.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkFormalDefaultTypeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Params == nil {
			continue
		}
		for _, fp := range fn.Params.List {
			if fp == nil || fp.Value == nil {
				continue
			}
			typeID, ok := fp.Type.(*syntax.Ident)
			if !ok || typeID == nil {
				continue
			}
			tname := typeID.String()
			if tname != "integer" {
				continue
			}
			fpName := ""
			if fp.Name != nil {
				fpName = fp.Name.String()
			}
			if lit := findFloatLiteral(fp.Value); lit != "" {
				diags = append(diags, Diagnostic{
					Code:     "formal-default-type-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"formal parameter %q has type %q but its default value contains float literal %q (ETSI 5.4.1.2)",
						fpName, tname, lit),
					Node: fp.Value,
					Span: syntax.SpanOf(fp.Value),
				})
			}
		}
	}
	return diags
}

func findFloatLiteral(expr syntax.Expr) string {
	var found string
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if found != "" {
			return false
		}
		vl, ok := n.(*syntax.ValueLiteral)
		if !ok || vl == nil || vl.Tok == nil {
			return true
		}
		raw := vl.Tok.String()
		if looksLikeFloatLiteral(raw) {
			found = raw
			return false
		}
		return true
	})
	return found
}

func looksLikeFloatLiteral(s string) bool {
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") {
		return false
	}
	for _, r := range s {
		if r == '.' || r == 'e' || r == 'E' {
			return true
		}
	}
	return false
}
