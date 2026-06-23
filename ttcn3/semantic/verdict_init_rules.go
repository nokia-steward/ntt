// verdict_init_rules.go enforces ETSI ES 201 873-1 clause 24:
//
//	Getverdict and setverdict operations shall only be
//	used in test cases, altsteps and functions.
//
// Module-level `const` initializers and module-control
// `const` initializers are NOT one of those contexts. The
// rule flags any `const ... := getverdict / setverdict(...)`
// at module top level.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkVerdictInitRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		vd, ok := d.Def.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			continue
		}
		if vd.KindTok.Kind() != syntax.CONST {
			continue
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Value == nil {
				continue
			}
			if name := findVerdictOpCall(dec.Value); name != "" {
				declared := ""
				if dec.Name != nil {
					declared = dec.Name.String()
				}
				diags = append(diags, Diagnostic{
					Code:     "module-level-verdict-op",
					Severity: SeverityError,
					Message: "module-level `const " + quoted(declared) +
						"` initializer uses `" + name +
						"`; verdict operations are only allowed in test cases, altsteps and functions (ETSI 24)",
					Node: dec.Value,
					Span: syntax.SpanOf(dec.Value),
				})
			}
		}
	}
	return diags
}

func findVerdictOpCall(expr syntax.Expr) string {
	var found string
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if found != "" {
			return false
		}
		switch v := n.(type) {
		case *syntax.CallExpr:
			if v == nil {
				return true
			}
			if id, ok := v.Fun.(*syntax.Ident); ok && id != nil &&
				isVerdictOpName(id.String()) {
				found = id.String()
				return false
			}
		case *syntax.Ident:
			if v != nil && isVerdictOpName(v.String()) {
				found = v.String()
				return false
			}
		}
		return true
	})
	return found
}

