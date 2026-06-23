// const_init_rules.go enforces a narrow flavour of ETSI ES
// 201 873-1 clause 10 "Constants" and clause 8.2.1 module
// parameters restriction f):
//
//	The initializer of a `const` or `modulepar`
//	declaration shall be a compile-time-known expression.
//	In particular, it must not call non-deterministic
//	built-ins; among those, `rnd(...)` is the canonical
//	case the conformance suite exercises.
//
// We deliberately keep the forbidden-call set short - just
// `rnd` for now - so the rule never fires on the many
// deterministic helper calls that legitimately appear in
// `const` initializers across the suite. We DO follow
// user-defined function calls transitively so that a
// modulepar like `modulepar integer X := f();` where `f()`
// internally calls `rnd()` is also flagged.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkConstInitRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	nondet := computeNonDeterministicFuncs(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			return true
		}
		var kind, kindClause string
		switch vd.KindTok.Kind() {
		case syntax.CONST:
			kind = "const"
			kindClause = "10"
		case syntax.MODULEPAR:
			kind = "modulepar"
			kindClause = "8.2.1"
		default:
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Value == nil {
				continue
			}
			if fn := findNonDeterministicCall(dec.Value, nondet); fn != "" {
				diags = append(diags, Diagnostic{
					Code:     kind + "-init-non-deterministic",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`%s` initializer must be compile-time deterministic; `%s(...)` is non-deterministic (ETSI %s)",
						kind, fn, kindClause),
					Node: dec.Value,
					Span: syntax.SpanOf(dec.Value),
				})
			}
		}
		return true
	})
	return diags
}

// computeNonDeterministicFuncs scans every function body in
// the module and returns the set of function names whose body
// (transitively) invokes a non-deterministic built-in. A
// shallow fixed-point is sufficient: we iterate until no new
// names are added.
func computeNonDeterministicFuncs(mod *syntax.Module) map[string]bool {
	// First pass: collect direct rnd / non-deterministic
	// call sites per function.
	directCalls := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.Name == nil {
			continue
		}
		name := fn.Name.String()
		set := map[string]bool{}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil {
				return true
			}
			id, ok := ce.Fun.(*syntax.Ident)
			if !ok || id == nil {
				return true
			}
			set[id.String()] = true
			return true
		})
		directCalls[name] = set
	}
	nondet := map[string]bool{}
	for name, calls := range directCalls {
		for c := range calls {
			if isNonDeterministicBuiltin(c) {
				nondet[name] = true
				break
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for name, calls := range directCalls {
			if nondet[name] {
				continue
			}
			for c := range calls {
				if nondet[c] {
					nondet[name] = true
					changed = true
					break
				}
			}
		}
	}
	return nondet
}

// findNonDeterministicCall walks an expression tree and returns
// the name of the first non-deterministic call it finds, or ""
// when none is present. A call is non-deterministic if its
// callee is either a non-deterministic built-in or a user
// function listed in `nondet`.
func findNonDeterministicCall(expr syntax.Expr, nondet map[string]bool) string {
	var found string
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if found != "" {
			return false
		}
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		name := id.String()
		if isNonDeterministicBuiltin(name) || nondet[name] {
			found = name
			return false
		}
		return true
	})
	return found
}

func isNonDeterministicBuiltin(name string) bool {
	switch name {
	case "rnd":
		return true
	}
	return false
}

// isVerdictOpName reports whether `name` is one of the verdict
// operations that ETSI 24 reserves for test cases, altsteps
// and functions. They must not appear in module-level
// initializers.
func isVerdictOpName(name string) bool {
	switch name {
	case "getverdict", "setverdict":
		return true
	}
	return false
}
