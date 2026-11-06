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
//
// Constants are runtime constants (clause 10): only those used,
// directly or through other constants, in type and array
// definitions are restricted (clause 10 b, 6.2.10.1).
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
	typeConsts := collectTypeDefConsts(mod)
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
			if kind == "const" && (dec.Name == nil || !typeConsts[dec.Name.String()]) {
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

// collectTypeDefConsts returns the names referenced from type and
// array definitions - field and declaration types, subtype
// constraints, array dimensions, enumerated values - closed over the
// initializers of the constants among them. Constants are matched by
// name.
func collectTypeDefConsts(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	add := func(n syntax.Node) {
		if n == nil {
			return
		}
		syntax.Inspect(n, func(m syntax.Node) bool {
			if id, ok := m.(*syntax.Ident); ok && id.Tok != nil {
				out[id.String()] = true
			}
			return m != nil
		})
	}
	addDims := func(dims []*syntax.ParenExpr) {
		for _, d := range dims {
			if d != nil {
				add(d)
			}
		}
	}
	inits := map[string][]syntax.Expr{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Field:
			add(x.Type)
			addDims(x.ArrayDef)
			if x.ValueConstraint != nil {
				add(x.ValueConstraint)
			}
			if x.LengthConstraint != nil {
				add(x.LengthConstraint)
			}
		case *syntax.EnumTypeDecl:
			for _, e := range x.Enums {
				if ce, ok := e.(*syntax.CallExpr); ok && ce.Args != nil {
					add(ce.Args)
				}
			}
		case *syntax.ValueDecl:
			add(x.Type)
			for _, dec := range x.Decls {
				if dec == nil {
					continue
				}
				addDims(dec.ArrayDef)
				if x.KindTok != nil && x.KindTok.Kind() == syntax.CONST && dec.Name != nil && dec.Value != nil {
					inits[dec.Name.String()] = append(inits[dec.Name.String()], dec.Value)
				}
			}
		case *syntax.TemplateDecl:
			add(x.Type)
		case *syntax.FormalPar:
			add(x.Type)
			addDims(x.ArrayDef)
		}
		return n != nil
	})
	// A constant used in a type definition shall itself refer only
	// to constants obeying the restriction.
	for changed := true; changed; {
		changed = false
		for name, vals := range inits {
			if !out[name] {
				continue
			}
			n := len(out)
			for _, v := range vals {
				add(v)
			}
			if len(out) != n {
				changed = true
			}
		}
	}
	return out
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
