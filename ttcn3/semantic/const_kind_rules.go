// const_kind_rules.go enforces ETSI ES 201 873-1 clause 10
// restrictions on `const` declarations:
//
//   - A constant shall not be of port type.
//   - A constant's value shall only be assigned once at the
//     declaration site; later `c := expr` statements are
//     forbidden.
//
// The two checks share a single walk of every `const T name`
// decl (top-level and inside function / testcase bodies).
// `const-port-type` flags port-typed constants;
// `const-reassign` flags later assignments to a const ident.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkConstKindRules(mod *syntax.Module) []Diagnostic {
	portTypes := collectPortTypes(mod)
	consts := collectConstNames(mod)

	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			if v == nil || v.KindTok == nil ||
				v.KindTok.Kind() != syntax.CONST ||
				len(portTypes) == 0 {
				return true
			}
			typeName := identName(v.Type)
			if typeName == "" {
				return true
			}
			if _, isPort := portTypes[typeName]; !isPort {
				return true
			}
			for _, dec := range v.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				// ETSI 10 (V4.11.1+) allows port-typed
				// constants only when the value is the
				// special `null`. Anything else is an
				// error.
				if isNullLiteral(dec.Value) {
					continue
				}
				diags = append(diags, Diagnostic{
					Code:     "const-port-type-not-null",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`const %s %s`: a port-typed constant must be initialised to `null` (ETSI 10)",
						typeName, dec.Name.String()),
					Node: dec,
					Span: syntax.SpanOf(dec),
				})
			}
		case *syntax.BinaryExpr:
			if v == nil || v.Op == nil ||
				v.Op.Kind() != syntax.ASSIGN {
				return true
			}
			name := identName(v.X)
			if name == "" || !consts[name] {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "const-reassign",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"cannot reassign constant %q (ETSI 10)",
					name),
				Node: v,
				Span: syntax.SpanOf(v),
			})
		}
		return true
	})
	return diags
}

// collectConstNames returns the set of every `const T name :=
// ...` identifier in the module - both top-level and inside
// function / testcase bodies.  The set is used by the
// const-reassign walker so subsequent `name := ...` statements
// can be rejected.
func collectConstNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	if mod == nil {
		return out
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.CONST {
			return true
		}
		for _, dec := range vd.Decls {
			if dec != nil && dec.Name != nil {
				out[dec.Name.String()] = true
			}
		}
		return true
	})
	return out
}
