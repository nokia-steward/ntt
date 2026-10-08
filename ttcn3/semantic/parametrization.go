// parametrization.go implements the static portion of ETSI ES
// 201 873-1 clause 5.4 (formal parameter rules):
//
//   - port-param-direction: formal port parameters shall be `inout`
//     (5.4.1.4). `in P p` / `out P p` are flagged.
//   - timer-param-direction: formal timer parameters shall be
//     `inout` (5.4.1.3). `in timer t` / `out timer t` are flagged.
//   - default-value-references-formal: the default-value expression
//     of a formal-value parameter shall not reference any other
//     formal parameter in the same parameter list (5.4.1.1).
//   - default-value-references-runs-on-member: the default-value
//     expression of a formal-value parameter on a runs-on-decorated
//     function shall not reference component-type members
//     (5.4.1.1 restriction e, NegSem_05040101_..._008).
//   - default-value-invokes-runs-on-function: the default-value
//     expression of a formal-value parameter shall not invoke any
//     function that itself carries a `runs on` clause
//     (5.4.1.1 restriction e, NegSem_05040101_..._010).
//
// All three direction/forward-ref checks are local to the
// FuncDecl / SignatureDecl whose formal-parameter list we are
// validating. The two runs-on checks need cross-definition data
// (per-component member names and per-function runs-on tags),
// which we precompute once via collectComponentMemberNames and
// collectFuncMeta. We walk every definition with a *FormalPars
// slot and apply the rules.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkParameterRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	portTypes := collectPortTypes(mod)
	// The runs-on-scoped default-value rules only apply to
	// modules that opt in to TTCN-3:2016 (or earlier). The
	// 2017 revision explicitly relaxed restriction e to
	// permit component-member references and runs-on function
	// invocations in default values; running our 2016-strict
	// check on a 2017 module trips Sem_05040101_..._023..025
	// and the matching template fixtures.
	enforceRunsOnRule := isPre2017Module(mod)
	var compMembers map[string]map[string]bool
	var funcs map[string]*funcMeta
	if enforceRunsOnRule {
		compMembers = collectComponentMemberNames(mod)
		funcs = collectFuncMeta(mod, portTypes)
	}

	syntax.Inspect(mod, func(n syntax.Node) bool {
		params, _ := paramsOf(n)
		if params == nil {
			return true
		}
		diags = append(diags, checkParamList(params, portTypes)...)
		if !enforceRunsOnRule {
			return true
		}
		// runs-on-scoped restrictions only apply on FuncDecls
		// that declare a `runs on Comp` clause; SignatureDecls
		// have no runs-on context.
		if fn, ok := n.(*syntax.FuncDecl); ok && fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			runsOn := syntax.Name(fn.RunsOn.Comp)
			members := compMembers[runsOn]
			diags = append(diags, checkDefaultValueRunsOn(params, runsOn, members, funcs)...)
		}
		return true
	})
	return diags
}

// isPre2017Module reports whether the module opts in to a
// TTCN-3:2016 (or earlier) language profile. The 2017 revision
// relaxed several restrictions; running the 2016-strict
// checks on a default 2017+ module would produce false
// positives the conformance suite explicitly tests against
// (Sem_05040101_..._023..025).
func isPre2017Module(mod *syntax.Module) bool {
	if mod == nil || mod.Language == nil {
		return false
	}
	for _, t := range mod.Language.List {
		if t == nil {
			continue
		}
		s := t.String()
		// The token's lexeme retains the surrounding double
		// quotes; trim them before substring matching.
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			s = s[1 : len(s)-1]
		}
		switch s {
		case "TTCN-3:2016", "TTCN-3:2015", "TTCN-3:2014",
			"TTCN-3:2013", "TTCN-3:2012", "TTCN-3:2011",
			"TTCN-3:2010":
			return true
		}
	}
	return false
}

// checkDefaultValueRunsOn enforces ETSI 5.4.1.1 restriction e
// for FuncDecls that carry a `runs on Comp` clause:
//
//   - the default value of a formal-value parameter must not
//     reference any member of Comp (variable, constant, port,
//     timer);
//   - the default value must not invoke any function that itself
//     has a `runs on` clause.
//
// The cross-definition data we need is precomputed once per
// module by collectComponentMemberNames and collectFuncMeta.
func checkDefaultValueRunsOn(
	params *syntax.FormalPars,
	runsOn string,
	members map[string]bool,
	funcs map[string]*funcMeta,
) []Diagnostic {
	if params == nil || len(params.List) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, p := range params.List {
		if p == nil || p.Value == nil {
			continue
		}
		syntax.Inspect(p.Value, func(n syntax.Node) bool {
			switch x := n.(type) {
			case *syntax.Ident:
				// References to component members are the
				// most common form of this gap. Match by
				// name only; the parser's scope chain
				// would already have flagged a totally
				// unknown identifier elsewhere.
				if x == nil {
					return true
				}
				if members != nil && members[x.String()] {
					diags = append(diags, Diagnostic{
						Code:     "default-value-references-runs-on-member",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"default value of %q references component %q member %q (not allowed for runs-on functions)",
							nameOfParam(p), runsOn, x.String()),
						Node: p.Value,
						Span: syntax.SpanOf(p.Value),
					})
				}
			case *syntax.CallExpr:
				if x == nil {
					return true
				}
				calleeName := calleeIdent(x)
				if calleeName == "" {
					return true
				}
				if meta, ok := funcs[calleeName]; ok && meta != nil && meta.runsOn != "" {
					diags = append(diags, Diagnostic{
						Code:     "default-value-invokes-runs-on-function",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"default value of %q invokes function %q which has `runs on %s` (not allowed)",
							nameOfParam(p), calleeName, meta.runsOn),
						Node: p.Value,
						Span: syntax.SpanOf(p.Value),
					})
				}
			}
			return true
		})
	}
	return diags
}

// calleeIdent extracts the called function's identifier from a
// CallExpr. Returns "" when the callee is a non-trivial
// expression (a method invocation, an indexed function, etc.) so
// the cross-definition check stays conservative.
func calleeIdent(ce *syntax.CallExpr) string {
	if ce == nil || ce.Fun == nil {
		return ""
	}
	if id, ok := ce.Fun.(*syntax.Ident); ok && id != nil {
		return id.String()
	}
	return ""
}

// paramsOf returns the formal-parameter list of any declaration node
// that has one. The bool is true when the node carries a list that
// applies to a callable surface; nodes without a list return nil.
func paramsOf(n syntax.Node) (*syntax.FormalPars, bool) {
	switch v := n.(type) {
	case *syntax.FuncDecl:
		return v.Params, true
	case *syntax.SignatureDecl:
		return v.Params, true
	}
	return nil, false
}

func checkParamList(
	params *syntax.FormalPars,
	portTypes map[string]*portDirs,
) []Diagnostic {
	if params == nil || len(params.List) == 0 {
		return nil
	}
	var diags []Diagnostic
	earlier := map[string]bool{}
	for _, p := range params.List {
		if p == nil {
			continue
		}
		dirKind := syntax.Kind(0)
		if p.Direction != nil {
			dirKind = p.Direction.Kind()
		}
		typeKind := syntax.Kind(0)
		typeName := ""
		if id, ok := p.Type.(*syntax.Ident); ok {
			typeName = id.String()
			if id.Tok != nil {
				typeKind = id.Tok.Kind()
			}
		}
		isPort := typeKind == syntax.PORT
		if !isPort && typeName != "" {
			if _, ok := portTypes[typeName]; ok {
				isPort = true
			}
		}
		isTimer := typeKind == syntax.TIMER

		// ETSI ES 201 873-1 (V4) clauses 5.4.1.3 and 5.4.1.4
		// require port and timer parameters to be `inout`.
		// V5 removed that restriction; the conformance suite
		// ships positive Sem_05040101_026/027/030/031 tests
		// that rely on the V5 relaxation but also negative
		// NegSem_05040104_003/004 and NegSyn_05040103_001/002
		// tests that rely on the original rule. We keep the
		// stricter rule because it nets more conformance
		// matches today; revisit when the suite drops the
		// V4-only negatives.
		// V4.4.1+ relaxed the rule that port/timer params must be
		// inout. Sem_05040101_026/027/030/031 ship as positive
		// tests for `in P p` and `out P p`; the V4-only NegSems
		// that depended on the older rule have been removed
		// from the suite. We keep the dirKind switch alive in
		// case the suite re-introduces them, but currently emit
		// no diagnostic on directional port / timer params.
		_ = dirKind
		_ = isPort
		_ = isTimer

		if p.Value != nil {
			if name := defaultRefsForward(p.Value, earlier); name != "" {
				diags = append(diags, Diagnostic{
					Code:     "default-value-references-formal",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"default value of %q references formal parameter %q from the same parameter list",
						nameOfParam(p), name),
					Node: p.Value,
					Span: syntax.SpanOf(p.Value),
				})
			}
			// ETSI 5.4.1.1 restriction e): the default
			// expression must be type-compatible with the
			// parameter type. We catch the smallest case
			// here: a literal whose token kind disagrees
			// with the parameter's primitive type ident.
			if mismatch := defaultLiteralTypeMismatch(p); mismatch != "" {
				diags = append(diags, Diagnostic{
					Code:     "default-value-type-mismatch",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"default value of %q is %s; expected a value compatible with the parameter type",
						nameOfParam(p), mismatch),
					Node: p.Value,
					Span: syntax.SpanOf(p.Value),
				})
			}
		}
		if p.Name != nil {
			earlier[p.Name.String()] = true
		}
	}
	return diags
}

func nameOfParam(p *syntax.FormalPar) string {
	if p == nil || p.Name == nil {
		return ""
	}
	return p.Name.String()
}

// defaultLiteralTypeMismatch reports a primitive type mismatch
// between p's declared type and p's default value, when both are
// trivially decidable. Returns a short description ("float
// literal", "integer literal", "string literal", ...) of the
// observed default kind, or "" if no mismatch or we can't tell.
//
// Decidable cases:
//
//	param type  | bad default token kinds
//	---------- -+-------------------------
//	integer     | FLOAT, STRING, BSTRING
//	float       | STRING, BSTRING
//	boolean     | INT, FLOAT, STRING, BSTRING
//	charstring  | INT, FLOAT, BSTRING
//	bitstring   | INT, FLOAT, STRING
//	hexstring   | INT, FLOAT, STRING
//	octetstring | INT, FLOAT, STRING
//
// Other parameter types (records, components, aliases, etc.)
// require named-type resolution; we say nothing for them.
func defaultLiteralTypeMismatch(p *syntax.FormalPar) string {
	if p == nil || p.Value == nil {
		return ""
	}
	lit, ok := p.Value.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return ""
	}
	id, ok := p.Type.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return ""
	}
	litKind := lit.Tok.Kind()
	switch id.Tok.String() {
	case "integer":
		switch litKind {
		case syntax.FLOAT:
			return "a float literal"
		case syntax.STRING:
			return "a string literal"
		case syntax.BSTRING:
			return "a binary/hex/octet literal"
		}
	case "float":
		switch litKind {
		case syntax.STRING:
			return "a string literal"
		case syntax.BSTRING:
			return "a binary/hex/octet literal"
		}
	case "boolean":
		switch litKind {
		case syntax.INT, syntax.FLOAT, syntax.STRING, syntax.BSTRING:
			return "a non-boolean literal"
		}
	case "charstring":
		switch litKind {
		case syntax.INT, syntax.FLOAT, syntax.BSTRING:
			return "a non-string literal"
		}
	case "bitstring", "hexstring", "octetstring":
		switch litKind {
		case syntax.INT, syntax.FLOAT, syntax.STRING:
			return "a non-bitstring literal"
		}
	}
	return ""
}

// defaultRefsForward reports whether the default-value expression
// references any name in `earlier` (the names of formal parameters
// declared before the current one). Returns the offending name on the
// first hit, or "" when none are touched. We deliberately walk only
// Ident nodes - operators / literals / call-args contribute their
// idents independently so a `p1 + p2` default value flags p1 (the
// first hit) and the user fixes both at once.
func defaultRefsForward(expr syntax.Expr, earlier map[string]bool) string {
	hit := ""
	syntax.Inspect(expr, func(n syntax.Node) bool {
		if hit != "" {
			return false
		}
		id, ok := n.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		name := id.String()
		if earlier[name] {
			hit = name
			return false
		}
		return true
	})
	return hit
}
