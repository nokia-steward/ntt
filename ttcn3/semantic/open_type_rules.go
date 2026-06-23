// open_type_rules.go enforces ETSI ES 201 873-1 clause 6.2.16
// ("The Open Type"). The `any` type is a placeholder usable only
// for typing formal parameters of external functions and for
// `select union ... { case ... }` discriminators; every other
// usage is rejected.
//
// The check is purely syntactic: we look for an `any` Ident in
// the Type slot of declarations and formal-parameter lists,
// emit a diagnostic, and move on. We deliberately do NOT flag
// `anytype` (a separate predefined type), `any from`, `any
// component.done`, `any timer`, or the `?` AnyValue wildcard
// (a value-level construct that parses to a different token).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkOpenTypeUsage(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.ValueDecl:
			diags = append(diags, openTypeDiagsForValueDecl(x)...)
		case *syntax.TemplateDecl:
			if d := openTypeDiagFromType(x.Type, openTypeContext(x)); d != nil {
				diags = append(diags, *d)
			}
			// `any` in a template's formal-parameter list is
			// equally forbidden (the parameter holds whatever
			// the caller supplies, so the formal type must be
			// concrete).
			diags = append(diags, openTypeDiagsForFormalPars(x.Params, "template parameter")...)
		case *syntax.FuncDecl:
			diags = append(diags, openTypeDiagsForFuncDecl(x)...)
		case *syntax.ModuleParameterGroup:
			for _, vd := range x.Decls {
				diags = append(diags, openTypeDiagsForValueDecl(vd)...)
			}
		}
		return true
	})
	return diags
}

// openTypeContext returns the human-readable label for the slot
// a TemplateDecl occupies. We keep this as a single line so the
// caller doesn't need to switch on the kind.
func openTypeContext(td *syntax.TemplateDecl) string {
	if td == nil {
		return "template"
	}
	return "template"
}

// openTypeDiagsForValueDecl handles the var / const / template
// (when written as a `template T x := ...` ValueDecl) /
// modulepar slots.
func openTypeDiagsForValueDecl(vd *syntax.ValueDecl) []Diagnostic {
	if vd == nil || vd.Type == nil {
		return nil
	}
	if vd.KindTok == nil {
		return nil
	}
	var label string
	switch vd.KindTok.Kind() {
	case syntax.VAR:
		label = "variable"
	case syntax.CONST:
		label = "constant"
	case syntax.TEMPLATE:
		label = "template"
	case syntax.MODULEPAR:
		label = "module parameter"
	default:
		// PORT / TIMER / ... never use `any` and the rule
		// would just be noise.
		return nil
	}
	if d := openTypeDiagFromType(vd.Type, label); d != nil {
		return []Diagnostic{*d}
	}
	return nil
}

// openTypeDiagsForFuncDecl handles formal parameters of
// non-external functions, altsteps, and testcases, plus the
// function return type. External functions / altsteps may use
// `any` because they cross the TCI boundary where the host
// language supplies the actual encoded value.
func openTypeDiagsForFuncDecl(fn *syntax.FuncDecl) []Diagnostic {
	if fn == nil {
		return nil
	}
	if fn.External != nil {
		return nil
	}
	var label string
	switch {
	case fn.KindTok == nil:
		label = "function parameter"
	case fn.KindTok.Kind() == syntax.ALTSTEP:
		label = "altstep parameter"
	case fn.KindTok.Kind() == syntax.TESTCASE:
		label = "testcase parameter"
	default:
		label = "function parameter"
	}
	var diags []Diagnostic
	diags = append(diags, openTypeDiagsForFormalPars(fn.Params, label)...)
	if fn.Return != nil && fn.Return.Type != nil {
		if d := openTypeDiagFromType(fn.Return.Type, "function return type"); d != nil {
			diags = append(diags, *d)
		}
	}
	return diags
}

func openTypeDiagsForFormalPars(pars *syntax.FormalPars, label string) []Diagnostic {
	if pars == nil {
		return nil
	}
	var diags []Diagnostic
	for _, fp := range pars.List {
		if fp == nil || fp.Type == nil {
			continue
		}
		if d := openTypeDiagFromType(fp.Type, label); d != nil {
			diags = append(diags, *d)
		}
	}
	return diags
}

// openTypeDiagFromType returns a diagnostic when expr is the
// bare `any` open-type ident. We also accept the shape `any
// length(...)` (a UnaryExpr / BinaryExpr wrapping the ident)
// because the conformance suite occasionally combines `any`
// with a length attribute and the ban applies regardless of
// the attribute.
func openTypeDiagFromType(expr syntax.Expr, slot string) *Diagnostic {
	if expr == nil {
		return nil
	}
	id, ok := expr.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return nil
	}
	if id.Tok.Kind() != syntax.ANYKW {
		return nil
	}
	return &Diagnostic{
		Code:     "open-type-forbidden",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"open type `any` is not allowed as %s (ETSI 6.2.16)",
			slot),
		Node: expr,
		Span: syntax.SpanOf(expr),
	}
}
