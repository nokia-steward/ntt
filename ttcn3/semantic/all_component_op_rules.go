// all_component_op_rules.go enforces ETSI ES 201 873-1 clauses
// 21.3.3 / 21.3.4 / 21.3.5 / 21.3.6: the special component
// reference `all component` (used as the receiver of
// `.stop`, `.kill`, `.killed`, `.done`, `.running`,
// `.alive`) is reserved for the MTC. A plain `function` is
// rejected here because it may legitimately be started on a
// PTC; testcases and module control run on the MTC and are
// left alone.
//
// The check walks every top-level FuncDecl / ControlPart
// body and inspects each `SelectorExpr` whose X is the
// `all component` ident.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAllComponentOpRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.KindTok == nil {
			continue
		}
		if fn.KindTok.Kind() != syntax.FUNCTION {
			continue
		}
		diags = append(diags, walkAllComponentOp(fn.Body)...)
	}
	return diags
}

func walkAllComponentOp(body *syntax.BlockStmt) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base == nil {
			return true
		}
		if base.String() != "all component" {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok || op == nil {
			return true
		}
		opName := op.String()
		switch opName {
		case "stop", "kill", "killed", "done", "running", "alive":
		default:
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "all-component-in-function",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`all component.%s` is only allowed from MTC (testcase or module control); a plain function may be started on a PTC (ETSI 21.3.x)",
				opName),
			Node: sel,
			Span: syntax.SpanOf(sel),
		})
		return true
	})
	return diags
}
