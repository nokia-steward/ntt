package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkStartArgTypeRules enforces ETSI 21.3.2: the function
// passed to `comp.start(f(...))` must not have any port-type or
// default-type formal parameters (directly or indirectly).
//
// The scan is two-phase:
//
//  1. Walk the module to find every function and altstep whose
//     own FormalPars list includes a port-type or default-type
//     argument, or a record/array of such, or an alias to such.
//  2. For every `<comp>.start(<funcRef>(...))` call, look up
//     funcRef's name in that table; if present, flag the start
//     site with a diagnostic.
//
// Indirect containment through record fields / array elements
// requires reading the named type. We keep it shallow: any type
// whose name itself appears in the "bad type" set propagates the
// taint to functions that take it.
func (a *Analyzer) checkStartArgTypeRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	badTypeNames := collectPortDefaultTaintedTypes(mod)
	badFns := collectStartUnsafeFuncs(mod, badTypeNames)
	if len(badFns) == 0 {
		return diags
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Fun == nil {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok || opIdent == nil || opIdent.String() != "start" {
			return true
		}
		if ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		fnCall, ok := ce.Args.List[0].(*syntax.CallExpr)
		if !ok || fnCall == nil || fnCall.Fun == nil {
			return true
		}
		fnIdent, ok := fnCall.Fun.(*syntax.Ident)
		if !ok || fnIdent == nil {
			return true
		}
		reason, bad := badFns[fnIdent.String()]
		if !bad {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "start-unsafe-arg-type",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`start(%s(...))` is forbidden: function parameter %s (ETSI 21.3.2)",
				fnIdent.String(), reason),
			Node: ce,
			Span: syntax.SpanOf(ce),
		})
		return true
	})
	return diags
}

// collectPortDefaultTaintedTypes returns the set of named types
// whose declared shape transitively involves a port or default
// type. Tracks aliases (TypeDecl), record-of (ListSpec /
// SetSpec), and record/struct fields.
func collectPortDefaultTaintedTypes(mod *syntax.Module) map[string]bool {
	bad := map[string]bool{"port": true, "default": true}
	// Seed: every user-declared port type is also bad,
	// since `start(f(<thatPort>))` is forbidden for the
	// same reason as the builtin `port` keyword.
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ptd, ok := n.(*syntax.PortTypeDecl)
		if !ok || ptd == nil || ptd.Name == nil {
			return true
		}
		bad[ptd.Name.String()] = true
		return true
	})
	// Saturate: each pass walks every type decl and marks
	// it bad if any of its named-type references is bad.
	// Fixpoint converges quickly because the type DAG in
	// the conformance suite is shallow.
	for changed := true; changed; {
		changed = false
		syntax.Inspect(mod, func(n syntax.Node) bool {
			switch v := n.(type) {
			case *syntax.SubTypeDecl:
				if v == nil || v.Field == nil || v.Field.Name == nil {
					return true
				}
				name := v.Field.Name.String()
				if bad[name] {
					return true
				}
				if typeSpecReferencesBad(v.Field.Type, bad) {
					bad[name] = true
					changed = true
				}
			case *syntax.StructTypeDecl:
				if v == nil || v.Name == nil || v.Fields == nil {
					return true
				}
				name := v.Name.String()
				if bad[name] {
					return true
				}
				for _, f := range v.Fields {
					if f == nil {
						continue
					}
					if typeSpecReferencesBad(f.Type, bad) {
						bad[name] = true
						changed = true
						break
					}
				}
			case *syntax.ComponentTypeDecl:
				// Component refs are NOT in the bad
				// set: passing a component reference
				// is allowed.
			case *syntax.PortTypeDecl:
				// Port type declarations themselves
				// don't introduce new bad names; the
				// `port` keyword usage in formal pars
				// is what we're after.
			}
			return true
		})
	}
	return bad
}

// typeSpecReferencesBad reports whether the type spec names a
// bad type, directly or via list/array element wrappers. Used
// from Field.Type / ListSpec.ElemType where the static type is
// syntax.TypeSpec, not the expression-typed forms.
func typeSpecReferencesBad(t syntax.TypeSpec, bad map[string]bool) bool {
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *syntax.RefSpec:
		if v == nil {
			return false
		}
		return typeExprReferencesBad(v.X, bad)
	case *syntax.ListSpec:
		if v == nil {
			return false
		}
		return typeSpecReferencesBad(v.ElemType, bad)
	}
	return false
}

// typeExprReferencesBad checks an expression-typed annotation
// (FormalPar.Type uses Expr, not TypeSpec).
func typeExprReferencesBad(t syntax.Expr, bad map[string]bool) bool {
	if t == nil {
		return false
	}
	if id, ok := t.(*syntax.Ident); ok && id != nil && id.Tok != nil {
		return bad[id.Tok.String()]
	}
	return false
}

// collectStartUnsafeFuncs returns the names of functions /
// altsteps whose own FormalPars list a port-type or default-type
// argument (directly or via a tainted alias). The map value
// describes WHY for a better diagnostic.
func collectStartUnsafeFuncs(mod *syntax.Module, badTypes map[string]bool) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Name == nil || fn.Params == nil {
			return true
		}
		for _, p := range fn.Params.List {
			if p == nil || p.Type == nil {
				continue
			}
			if id, ok := p.Type.(*syntax.Ident); ok && id != nil && id.Tok != nil {
				name := id.Tok.String()
				if badTypes[name] {
					out[fn.Name.String()] = fmt.Sprintf(
						"%q has port/default type", name)
					return true
				}
			}
			if typeExprReferencesBad(p.Type, badTypes) {
				out[fn.Name.String()] = "parameter type contains port/default"
				return true
			}
		}
		return true
	})
	return out
}
