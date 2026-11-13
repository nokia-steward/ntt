// const_literal_type.go implements ETSI ES 201 873-1 clause 6.1.1
// type-checking for `const <basic-type> name := <literal>` and
// `modulepar <basic-type> name := <literal>` declarations.
//
// We only model the case where the basic type is one of the eight
// built-in primitive types and the initialiser is a literal value
// whose kind we can recognise from the token alone. This catches
// the NegSyn_060101_TopLevel_* family that assigns one primitive
// literal to another primitive type (e.g. `const bitstring := "x"`).
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkConstLiteralTypes(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd.KindTok == nil {
			return true
		}
		switch vd.KindTok.Kind() {
		case syntax.CONST, syntax.MODULEPAR:
		default:
			return true
		}
		want := basicTypeNameOf(vd.Type)
		if want == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Value == nil {
				continue
			}
			lit, ok := dec.Value.(*syntax.ValueLiteral)
			if !ok || lit.Tok == nil {
				continue
			}
			got := literalTypeName(lit)
			if got == "" || got == want {
				continue
			}
			// Bitstring / hexstring / octetstring share the
			// BSTRING token; compare via suffix.
			if want == got {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "const-literal-type-mismatch",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"cannot assign %s literal to %s constant %q",
					got, want, syntax.Name(dec.Name)),
				Node: dec,
				Span: syntax.SpanOf(dec),
			})
		}
		return true
	})
	return diags
}

func basicTypeNameOf(e syntax.Expr) string {
	id, ok := e.(*syntax.Ident)
	if !ok || id.Tok == nil {
		return ""
	}
	switch id.String() {
	case "bitstring", "hexstring", "octetstring",
		"charstring", "universal", "boolean",
		"integer", "float":
		return id.String()
	}
	return ""
}

func literalTypeName(v *syntax.ValueLiteral) string {
	if v == nil || v.Tok == nil {
		return ""
	}
	switch v.Tok.Kind() {
	case syntax.INT:
		return "integer"
	case syntax.FLOAT:
		return "float"
	case syntax.STRING:
		return "charstring"
	case syntax.TRUE, syntax.FALSE:
		return "boolean"
	case syntax.BSTRING:
		s := v.Tok.String()
		if len(s) < 3 {
			return ""
		}
		switch strings.ToUpper(s[len(s)-1:]) {
		case "B":
			return "bitstring"
		case "H":
			return "hexstring"
		case "O":
			return "octetstring"
		}
	}
	return ""
}
