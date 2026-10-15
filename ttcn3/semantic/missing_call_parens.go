// missing_call_parens.go implements ETSI ES 201 873-1 clause 5.4.2:
// invocations of functions, testcases, altsteps and signatures must
// always use parentheses, even when the formal parameter list is
// empty. Using the bare identifier yields a function value, which
// TTCN-3 does not support.
//
// The check catches the NegSem_050402_actual_parameters_115..117
// family.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkMissingCallParens(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	// Collect names of function-like entities and the Ident*
	// pointers that name their declarations (to suppress
	// self-reference false positives).
	funcs := map[string]string{} // name -> kind label
	declNames := map[*syntax.Ident]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.FuncDecl:
			if x.Name == nil || x.KindTok == nil {
				continue
			}
			switch x.KindTok.Kind() {
			case syntax.FUNCTION:
				funcs[x.Name.String()] = "function"
			case syntax.TESTCASE:
				funcs[x.Name.String()] = "testcase"
			case syntax.ALTSTEP:
				funcs[x.Name.String()] = "altstep"
			}
			declNames[x.Name] = true
		case *syntax.SignatureDecl:
			// Signatures are not invoked - they are used as
			// type qualifiers in `port.call(sig:template, ...)`
			// templates. Bare reference is legitimate.
			if x.Name != nil {
				declNames[x.Name] = true
			}
		}
	}
	if len(funcs) == 0 {
		return nil
	}

	// First pass: collect Idents that appear as the Fun of a
	// CallExpr (legitimate invocation site).
	calleeSet := map[*syntax.Ident]bool{}
	// Idents inside `[] a_test {` alt-guard expression
	// statements - we'll flag these separately because they
	// parse as ExprStmt+Ident.
	altGuardBareIdents := map[*syntax.Ident]string{}

	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		if ce, ok := n.(*syntax.CallExpr); ok && ce.Fun != nil {
			if id, ok := ce.Fun.(*syntax.Ident); ok {
				calleeSet[id] = true
			}
		}
		if cc, ok := n.(*syntax.CommClause); ok && cc.Comm != nil {
			if es, ok := cc.Comm.(*syntax.ExprStmt); ok && es != nil {
				if id, ok := es.Expr.(*syntax.Ident); ok && id.Tok != nil {
					if kind, ok := funcs[id.String()]; ok && kind == "altstep" {
						altGuardBareIdents[id] = kind
					}
				}
			}
		}
		return true
	})

	// Second pass: flag every Ident naming a function-like
	// entity that isn't on the callee allow-list and isn't its
	// own declaration name.
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if n == nil {
			return true
		}
		id, ok := n.(*syntax.Ident)
		if !ok || id.Tok == nil {
			return true
		}
		kind, ok := funcs[id.String()]
		if !ok {
			return true
		}
		if calleeSet[id] || declNames[id] {
			return true
		}
		// Already-flagged alt-guard bare identifier - emit
		// the diagnostic here so we don't double-report.
		if _, isAlt := altGuardBareIdents[id]; isAlt {
			diags = append(diags, Diagnostic{
				Code:     "missing-call-parens",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s %q invocation requires empty parentheses",
					kind, id.String()),
				Node: id,
				Span: syntax.SpanOf(id),
			})
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "missing-call-parens",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s %q invocation requires empty parentheses",
				kind, id.String()),
			Node: id,
			Span: syntax.SpanOf(id),
		})
		return true
	})

	return diags
}
