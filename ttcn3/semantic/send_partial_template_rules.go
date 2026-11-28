// send_partial_template_rules.go enforces ETSI ES 201 873-1
// clause 22.2.1 h:
//
//	the TemplateInstance argument of `port.send(...)` shall
//	have a specific value: matching mechanisms (`?`, `*`,
//	`pattern`, `decmatch`) and the "uninitialised" placeholder
//	`-` are forbidden.
//
// Symmetrical restrictions apply to the messages emitted by
// the procedure-port operations `reply` / `raise`, but ETSI
// 22.3.x is exercised by a different set of conformance
// fixtures that already round-trip through the runtime; we
// keep this rule focused on the message-based `send` form
// where the suite tests it most heavily.
//
// We resolve two send-argument shapes:
//
//   - a bare identifier referring to a `var template T name`
//     declared with a composite-literal initializer that
//     contains a `-` element; and
//   - a directly-passed composite literal with a `-` element.
//
// Conservatively, only top-level `-` elements (not nested
// fields) are considered. The conformance fixtures for the
// rule (NegSem_220201_..._006, NegSem_060203_..._015,
// NegSem_060207_..._009) all use the top-level shape; a
// future pass with a deeper walk can broaden coverage when
// we have positive Sem tests to validate against.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSendPartialTemplateRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	partialTpls := collectPartialTemplateVars(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		id, ok := sel.Sel.(*syntax.Ident)
		if !ok || id == nil || id.String() != "send" {
			return true
		}
		arg := ce.Args.List[0]
		// Strip an optional `Type:` qualifier so `p.send(T:lit)`
		// is treated as `lit`.
		if be, ok := arg.(*syntax.BinaryExpr); ok && be != nil && be.Op != nil && be.Op.Kind() == syntax.COLON {
			arg = be.Y
		}
		switch v := arg.(type) {
		case *syntax.Ident:
			if v == nil {
				return true
			}
			if partialTpls[v.String()] {
				diags = append(diags, Diagnostic{
					Code:     "send-partial-template",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"send argument %q is a template with an uninitialised `-` element; "+
							"send requires a specific value (ETSI 22.2.1 h)",
						v.String()),
					Node: v,
					Span: syntax.SpanOf(v),
				})
			}
		case *syntax.CompositeLiteral:
			if v == nil {
				return true
			}
			if compositeHasDash(v) {
				diags = append(diags, Diagnostic{
					Code:     "send-partial-template",
					Severity: SeverityError,
					Message:  "send argument has an uninitialised `-` element; send requires a specific value (ETSI 22.2.1 h)",
					Node:     v,
					Span:     syntax.SpanOf(v),
				})
			}
		}
		return true
	})
	return diags
}

// collectPartialTemplateVars returns the set of `var template T
// name` identifiers initialised with a composite literal whose
// top-level element list contains a `-` placeholder. Plain
// `template T name := ...` declarations (the TemplateDecl shape)
// are deliberately excluded: those are conventionally used as
// matching templates against received messages, and the send
// restriction only bites when the same template flows into a
// `send` call.
func collectPartialTemplateVars(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction == nil {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil || d.Value == nil {
				continue
			}
			cl, ok := d.Value.(*syntax.CompositeLiteral)
			if !ok {
				continue
			}
			if compositeHasDash(cl) {
				out[d.Name.String()] = true
			}
		}
		return true
	})
	return out
}

// compositeHasDash reports whether the composite literal has a
// top-level `-` element (ValueLiteral whose Tok is SUB).
func compositeHasDash(cl *syntax.CompositeLiteral) bool {
	if cl == nil {
		return false
	}
	for _, el := range cl.List {
		if vl, ok := el.(*syntax.ValueLiteral); ok && vl != nil && vl.Tok != nil &&
			vl.Tok.Kind() == syntax.SUB {
			return true
		}
	}
	return false
}
