// component_test_op_rules.go enforces a few narrow ETSI 21.3.7 /
// 21.3.8 restrictions that the conformance suite hits hard:
//
//   - `done` / `killed` cannot be applied to a component type name;
//     only to a PTC component reference (NegSem_210307_001,
//     NegSem_210308_001).
//   - `any component.done` / `all component.done` (and the same
//     for `killed`) cannot carry a `-> value` verdict redirect
//     (NegSem_210307_008/009, NegSem_210308_008/009).
//
// The rule is implemented in this file rather than in
// component_op_receiver.go because it needs the RedirectExpr
// structure, which the receiver-kind check does not look at.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

var componentTestOpsWithRedirect = map[string]bool{
	"done":   true,
	"killed": true,
}

func (a *Analyzer) checkComponentTestOpRules(mod *syntax.Module) []Diagnostic {
	declKinds := collectDeclaredTypeKinds(mod)
	if len(declKinds) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectVarDeclaredTypeNames(fn.Body)
		paramTypes := collectFormalParamTypeNames(fn.Params)
		diags = append(diags, doneRedirectTypeDiags(fn.Body, varTypes, paramTypes)...)
	}
	diags = append(diags, bareComponentTestOpBlockDiags(mod)...)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		// any/all component.done -> value <X> is forbidden.
		re, ok := n.(*syntax.RedirectExpr)
		if ok && re != nil && len(re.Value) > 0 {
			diags = appendDiagsIfAnyAllRedirect(diags, re)
		}
		// <TypeName>.done / <TypeName>.killed - using the
		// type name directly is illegal; only a PTC instance
		// reference is allowed.
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok {
			return true
		}
		op := opIdent.String()
		if !componentTestOpsWithRedirect[op] {
			return true
		}
		recv, ok := sel.X.(*syntax.Ident)
		if !ok {
			return true
		}
		name := recv.String()
		if kind, known := declKinds[name]; known && kind == tkComponent {
			diags = append(diags, Diagnostic{
				Code:     "component-test-op-on-type-name",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s applied to component type %q: use a component reference (ETSI 21.3.7/21.3.8)",
					op, name),
				Node: sel,
				Span: syntax.SpanOf(sel),
			})
		}
		return true
	})
	return diags
}

func bareComponentTestOpBlockDiags(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		blk, ok := n.(*syntax.BlockStmt)
		if !ok || blk == nil {
			return true
		}
		for i := 0; i+1 < len(blk.Stmts); i++ {
			es, ok := blk.Stmts[i].(*syntax.ExprStmt)
			if !ok || es == nil {
				continue
			}
			id, ok := es.Expr.(*syntax.Ident)
			if !ok || id == nil || !componentTestOpsWithRedirect[id.String()] {
				continue
			}
			if _, ok := blk.Stmts[i+1].(*syntax.BlockStmt); !ok {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "component-test-op-missing-receiver",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`%s` requires an explicit component receiver, for example `ptc.%s` or `any component.%s` (ETSI 21.3.7/21.3.8)",
					id.String(), id.String(), id.String()),
				Node: es,
				Span: syntax.SpanOf(es),
			})
		}
		return true
	})
	return diags
}

// appendDiagsIfAnyAllRedirect flags `any component.<op> -> value X`
// or `all component.<op> -> value X` where <op> is `done` or
// `killed`. The receiver `X` of the redirect is the underlying
// done/killed call we want to inspect.
func appendDiagsIfAnyAllRedirect(diags []Diagnostic, re *syntax.RedirectExpr) []Diagnostic {
	target := re.X
	for {
		ce, ok := target.(*syntax.CallExpr)
		if !ok {
			break
		}
		target = ce.Fun
	}
	sel, ok := target.(*syntax.SelectorExpr)
	if !ok {
		return diags
	}
	opIdent, ok := sel.Sel.(*syntax.Ident)
	if !ok {
		return diags
	}
	op := opIdent.String()
	if !componentTestOpsWithRedirect[op] {
		return diags
	}
	if !isAnyAllComponentReceiver(sel.X) {
		return diags
	}
	return append(diags, Diagnostic{
		Code:     "component-test-op-redirect-on-any-all",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"`-> value` redirect is not allowed on `any/all component.%s` (ETSI 21.3.7 / 21.3.8)",
			op),
		Node: re,
		Span: syntax.SpanOf(re),
	})
}

// doneRedirectTypeDiags flags `.done -> value X` (and the same
// for `.killed`) when X has a known type other than
// `verdicttype`. Unknown / opaque types are skipped: the rule's
// goal is to catch the obvious mistakes the suite throws at us
// (NegSem_210307_007, NegSem_210308_007).
func doneRedirectTypeDiags(
	body *syntax.BlockStmt,
	varTypes, paramTypes map[string]string,
) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(body, func(n syntax.Node) bool {
		re, ok := n.(*syntax.RedirectExpr)
		if !ok || re == nil || len(re.Value) == 0 {
			return true
		}
		target := re.X
		for {
			ce, ok := target.(*syntax.CallExpr)
			if !ok {
				break
			}
			target = ce.Fun
		}
		sel, ok := target.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok || !componentTestOpsWithRedirect[opIdent.String()] {
			return true
		}
		for _, v := range re.Value {
			name := identName(v)
			if name == "" {
				continue
			}
			typ, known := varTypes[name]
			if !known {
				typ, known = paramTypes[name]
			}
			if !known || typ == "" {
				continue
			}
			if typ == "verdicttype" {
				continue
			}
			diags = append(diags, Diagnostic{
				Code:     "component-test-op-redirect-wrong-type",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`-> value %s` on `.%s` requires %q to be of type `verdicttype`, got %q (ETSI 21.3.7/21.3.8)",
					name, opIdent.String(), name, typ),
				Node: v,
				Span: syntax.SpanOf(v),
			})
		}
		return true
	})
	return diags
}

// isAnyAllComponentReceiver matches the `any component` /
// `all component` qualifier. The parser surfaces these as a
// single Ident node carrying two tokens (Tok / Tok2), and we
// also accept the two-Ident BinaryExpr fallback shape just in
// case a future parser revision splits them.
func isAnyAllComponentReceiver(e syntax.Expr) bool {
	switch v := e.(type) {
	case *syntax.Ident:
		if v == nil || v.Tok == nil || v.Tok2 == nil {
			return false
		}
		first := v.Tok.String()
		second := v.Tok2.String()
		return (first == "any" || first == "all") && second == "component"
	case *syntax.BinaryExpr:
		if v == nil {
			return false
		}
		head, ok := v.X.(*syntax.Ident)
		if !ok || head == nil {
			return false
		}
		s := head.String()
		if s != "any" && s != "all" {
			return false
		}
		tail, ok := v.Y.(*syntax.Ident)
		return ok && tail != nil && tail.String() == "component"
	}
	return false
}
