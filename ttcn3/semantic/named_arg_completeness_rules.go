// named_arg_completeness_rules.go enforces ETSI ES 201 873-1
// clause 5.4.2: when a call uses assignment (named-argument)
// notation, every formal parameter that has no default value must
// still receive an actual parameter.
//
//	function f(integer p1, integer p2 := 0, integer p3) { ... }
//	f(p2 := 5, p3 := 7);   // ERROR: p1 has no default and is omitted
//
// The rule only fires once a call mixes in at least one `name :=
// value` argument, because that is the only shape where a no-default
// formal can be silently skipped. Pure positional calls are governed
// by the ordinary arity rules elsewhere.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkNamedArgCompletenessRules implements the clause 5.4.2 named
// argument completeness check.
func (a *Analyzer) checkNamedArgCompletenessRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	funcs := collectFuncDeclsByName(mod)
	if len(funcs) == 0 {
		return nil
	}

	var diags []Diagnostic
	mod.Inspect(func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || call.Args == nil {
			return true
		}
		id, ok := call.Fun.(*syntax.Ident)
		if !ok {
			return true
		}
		fd, ok := funcs[id.String()]
		if !ok || fd.Params == nil {
			return true
		}
		diags = append(diags, checkOneNamedCall(call, fd)...)
		return true
	})
	return diags
}

// checkOneNamedCall validates a single CallExpr against the callee's
// formal parameter list. Returns one diagnostic per no-default formal
// that the call omits. Only fires when the call uses named notation.
func checkOneNamedCall(call *syntax.CallExpr, fd *syntax.FuncDecl) []Diagnostic {
	provided := map[string]bool{}
	positional := 0
	hasNamed := false
	for _, arg := range call.Args.List {
		if name, ok := namedArgName(arg); ok {
			provided[name] = true
			hasNamed = true
			continue
		}
		positional++
	}
	if !hasNamed {
		// Pure positional call: handled by ordinary arity rules.
		return nil
	}

	var diags []Diagnostic
	for i, p := range fd.Params.List {
		if p == nil || p.Name == nil {
			continue
		}
		// A formal with a default value (`:= expr`) may be omitted.
		if p.Value != nil {
			continue
		}
		// Covered by an explicit named argument.
		if provided[p.Name.String()] {
			continue
		}
		// Covered by a leading positional argument (positional
		// arguments fill the first slots, in order, per 5.4.2).
		if i < positional {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "named-arg-missing-mandatory",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"missing actual parameter for %q: a formal parameter without a default value must be supplied in assignment notation (ETSI 5.4.2)",
				p.Name.String()),
			Node: call,
			Span: syntax.SpanOf(call),
		})
	}
	return diags
}

// namedArgName returns the formal-parameter name targeted by an
// assignment-notation actual argument (`name := value`) and true, or
// ("", false) for a positional argument.
func namedArgName(arg syntax.Expr) (string, bool) {
	b, ok := arg.(*syntax.BinaryExpr)
	if !ok || b.Op == nil || b.Op.Kind() != syntax.ASSIGN {
		return "", false
	}
	id, ok := b.X.(*syntax.Ident)
	if !ok || id == nil {
		return "", false
	}
	return id.String(), true
}

// collectFuncDeclsByName indexes every function / testcase / altstep
// declaration in the module by its name. Signatures and templates are
// intentionally excluded - their call sites have different shapes.
func collectFuncDeclsByName(mod *syntax.Module) map[string]*syntax.FuncDecl {
	out := map[string]*syntax.FuncDecl{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fd, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fd == nil || fd.Name == nil {
			continue
		}
		out[fd.Name.String()] = fd
	}
	return out
}
