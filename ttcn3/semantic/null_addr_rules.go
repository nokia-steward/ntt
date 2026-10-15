// null_addr_rules.go enforces ETSI ES 201 873-1 clauses 22.2.2 /
// 22.2.3 / 22.3.x:
//
//	"No AddressRef shall contain the special value `null` at the
//	 time of the operation."
//
// applied to the `from <addr>` clause of receive / trigger /
// getcall / getreply / catch / check operations AND the `to
// <addr>` clause of send / call / reply / raise. We catch:
//
//   - The bare `null` literal in either clause:
//       `p.call(S:{}, nowait) to null`
//   - A variable that is statically initialised to `null` and
//     never reassigned in the enclosing function body.
//   - Either of the above inside a parenthesised multicast list,
//     e.g. `to (mtc, v_compRef)` where v_compRef is statically null.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkNullAddressInFromClause(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		nullVars := collectStaticallyNullVars(fn.Body)
		// We need to walk every from/to clause even when the
		// function has no null-initialised vars, because a
		// bare `null` literal in the clause is itself a
		// violation.
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			be, ok := sn.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil {
				return true
			}
			var clause string
			switch be.Op.Kind() {
			case syntax.FROM:
				clause = "from"
			case syntax.TO:
				clause = "to"
			default:
				return true
			}
			for _, item := range addrRefItems(be.Y) {
				if isNullLiteral(item) {
					diags = append(diags, Diagnostic{
						Code:     fromOrToCode(clause),
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%s null: address reference shall not be `null` at the time of the operation (ETSI 22.2.2)",
							clause),
						Node: item,
						Span: syntax.SpanOf(item),
					})
					continue
				}
				name := identName(item)
				if name == "" {
					continue
				}
				if nullVars[name] {
					diags = append(diags, Diagnostic{
						Code:     fromOrToCode(clause),
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%s %s: address reference is statically `null` (ETSI 22.2.2)",
							clause, name),
						Node: item,
						Span: syntax.SpanOf(item),
					})
				}
			}
			return true
		})
		return false
	})
	return diags
}

// fromOrToCode picks the diagnostic-code suffix matching the
// clause; we keep them distinct so downstream tooling can tell
// the two apart without parsing the message text.
func fromOrToCode(clause string) string {
	if clause == "to" {
		return "to-clause-null-address"
	}
	return "from-clause-null-address"
}

// addrRefItems unwraps the address-list shapes the parser
// produces for `from`/`to` clauses and returns the individual
// address-reference expressions. The shapes handled are:
//
//   - bare ident / null / mtc / system / self  -> [x]
//   - paren list `(a, b, c)`                    -> [a, b, c]
//   - single-element paren `(a)`                -> [a]
//
// Anything else is returned as a single-element slice so the
// caller can still inspect it; unknown shapes simply produce no
// diagnostics because identName / isNullLiteral fall through.
func addrRefItems(e syntax.Expr) []syntax.Expr {
	if e == nil {
		return nil
	}
	if pe, ok := e.(*syntax.ParenExpr); ok && pe != nil {
		if len(pe.List) == 0 {
			return nil
		}
		return pe.List
	}
	return []syntax.Expr{e}
}

// collectStaticallyNullVars returns the set of variable names in
// body whose declared initializer is the bare `null` literal and
// whose name never appears on the LHS of an assignment in the
// same body. Reassignment via field access, indexed write, or
// in/inout passing is conservatively ignored - we still flag
// them; the rule treats the var as `null` for purposes of static
// reasoning.
func collectStaticallyNullVars(body syntax.Node) map[string]bool {
	candidates := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil || dec.Value == nil {
				continue
			}
			if isNullLiteral(dec.Value) {
				candidates[dec.Name.String()] = true
			}
		}
		return true
	})
	if len(candidates) == 0 {
		return candidates
	}
	// Drop any candidate that is reassigned via plain `x := ...`.
	syntax.Inspect(body, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		if name := identName(be.X); name != "" {
			delete(candidates, name)
		}
		return true
	})
	// Drop any candidate that is written to by a port-operation
	// redirect: `-> param(x, ...)`, `-> value x`, or
	// `sender x`. The receiver-side semantics of those clauses
	// are an out-store to the named variable, so the static
	// `null` assumption no longer holds.
	syntax.Inspect(body, func(n syntax.Node) bool {
		re, ok := n.(*syntax.RedirectExpr)
		if !ok || re == nil {
			return true
		}
		for _, v := range re.Value {
			if name := identName(v); name != "" {
				delete(candidates, name)
			}
		}
		for _, p := range re.Param {
			if name := identName(p); name != "" {
				delete(candidates, name)
			}
		}
		if name := identName(re.Sender); name != "" {
			delete(candidates, name)
		}
		return true
	})
	return candidates
}

func isNullLiteral(e syntax.Expr) bool {
	v, ok := e.(*syntax.ValueLiteral)
	if !ok || v == nil || v.Tok == nil {
		return false
	}
	return v.Tok.Kind() == syntax.NULL
}

// fromTargetName extracts the bare identifier from the `Y` of a
// `from Y` binary expression. Multicast / list forms are handled
// by addrRefItems; this helper stays around for callers that
// already expect the single-name shape.
func fromTargetName(e syntax.Expr) string {
	return identName(e)
}
