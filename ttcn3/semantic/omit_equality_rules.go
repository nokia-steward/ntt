// omit_equality_rules.go enforces ETSI ES 201 873-1
// clause 7.1.3: the \`omit\` symbol is neither a value
// nor a field reference and therefore cannot appear as
// an operand of the equality (\`==\`) or inequality
// (\`!=\`) operators. To test whether an optional field
// holds a value, \`ispresent\` should be used instead.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkOmitEqualityRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil {
			return true
		}
		if be.Op.Kind() != syntax.EQ && be.Op.Kind() != syntax.NE {
			return true
		}
		if !operandIsOmit(be.X) && !operandIsOmit(be.Y) {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "omit-as-equality-operand",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`omit` is neither a value nor a field reference; it cannot appear as an operand of `%s` (ETSI 7.1.3); use `ispresent` instead",
				be.Op.String()),
			Node: be,
			Span: syntax.SpanOf(be),
		})
		return true
	})
	return diags
}

func operandIsOmit(e syntax.Expr) bool {
	if id, ok := e.(*syntax.Ident); ok && id != nil && id.Tok != nil &&
		id.Tok.Kind() == syntax.OMIT {
		return true
	}
	if vl, ok := e.(*syntax.ValueLiteral); ok && vl != nil && vl.Tok != nil &&
		vl.Tok.Kind() == syntax.OMIT {
		return true
	}
	return false
}
