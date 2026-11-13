// raise_operation_rules.go enforces a small subset of ETSI ES 201
// 873-1 clause 22.3.5 (the `raise` procedure-port operation):
//
//   - the signature named as the first argument must declare an
//     `exception(...)` list, because `raise` carries that exception
//     value across the port (NegSem_220305_raise_operation_002).
//
// The check is purely structural: we scan every signature decl in
// the current module, remember which ones have an exception list,
// and flag every `<port>.raise(<sigName>, ...)` whose first
// argument resolves to a signature that doesn't.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRaiseOperationRules(mod *syntax.Module) []Diagnostic {
	sigsWithExc := collectSignatureExceptionFlags(mod)
	if len(sigsWithExc) == 0 {
		return nil
	}
	sigExcTypes := collectSignatureExceptionTypes(mod)
	varTypes := collectModuleVarTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok || opIdent.String() != "raise" {
			return true
		}
		if ce.Args == nil || len(ce.Args.List) == 0 {
			return true
		}
		firstArg := ce.Args.List[0]
		if firstArg == nil {
			return true
		}
		var sigName string
		switch arg := firstArg.(type) {
		case *syntax.Ident:
			sigName = arg.String()
		case *syntax.BinaryExpr:
			// `S:expr` shape - the first operand is the
			// signature reference.
			if id, ok := arg.X.(*syntax.Ident); ok {
				sigName = id.String()
			}
		}
		if sigName == "" {
			return true
		}
		hasExc, known := sigsWithExc[sigName]
		if known && !hasExc {
			diags = append(diags, Diagnostic{
				Code:     "raise-on-signature-without-exceptions",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"raise(%s, ...): signature %q has no exception list (ETSI 22.3.5)",
					sigName, sigName),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
		}
		// The exception value (second arg of raise) must
		// conform to the template(value) restriction (22.3.5
		// f). We flag obvious matching mechanisms; the deep
		// wildcard check is shared with the signature
		// template rule.
		if len(ce.Args.List) >= 2 {
			expr := ce.Args.List[1]
			typeQual := ""
			if be, ok := expr.(*syntax.BinaryExpr); ok && be.Op != nil &&
				be.Op.String() == ":" {
				typeQual = identName(be.X)
				expr = be.Y
			}
			if mech := deepMatchingMechanism(expr, nil); mech != "" {
				diags = append(diags, Diagnostic{
					Code:     "raise-exception-not-specific",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"raise(%s, ...): exception value uses %s, but template(value) is required (ETSI 22.3.5 f)",
						sigName, mech),
					Node: ce.Args.List[1],
					Span: syntax.SpanOf(ce.Args.List[1]),
				})
			}
			if excList, ok := sigExcTypes[sigName]; ok && len(excList) > 0 {
				valueTy := typeQual
				if valueTy == "" {
					valueTy = inferRaiseValueType(expr, varTypes)
				}
				if valueTy != "" {
					match := false
					for _, t := range excList {
						if t == valueTy {
							match = true
							break
						}
					}
					if !match {
						diags = append(diags, Diagnostic{
							Code:     "raise-exception-type-not-in-list",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"raise(%s, ...): exception value type %q is not in the signature's exception list %v (ETSI 22.3.5)",
								sigName, valueTy, excList),
							Node: ce.Args.List[1],
							Span: syntax.SpanOf(ce.Args.List[1]),
						})
					}
				}
			}
		}
		return true
	})
	return diags
}

// inferRaiseValueType guesses the static type identifier of an
// exception value passed to raise(). Returns "" when the shape
// isn't statically resolvable. We only handle the common cases:
// bare idents (look up varTypes) and literal values - the latter
// routed through the existing literalTypeName helper.
func inferRaiseValueType(e syntax.Expr, varTypes map[string]string) string {
	switch x := e.(type) {
	case *syntax.Ident:
		if x == nil || x.Tok == nil {
			return ""
		}
		if ty, ok := varTypes[x.String()]; ok {
			return stripArraySuffix(ty)
		}
	case *syntax.ValueLiteral:
		return literalTypeName(x)
	}
	return ""
}

// collectSignatureExceptionTypes maps each signature name to the
// list of declared exception type identifiers, preserving order.
func collectSignatureExceptionTypes(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd.Name == nil || sd.Exception == nil {
			continue
		}
		var list []string
		for _, e := range sd.Exception.List {
			if ty := identName(e); ty != "" {
				list = append(list, ty)
			}
		}
		if len(list) > 0 {
			out[sd.Name.String()] = list
		}
	}
	return out
}

// collectSignatureExceptionFlags maps each module-level signature
// name to a bool indicating whether it declared an `exception(...)`
// list. Signatures missing from the map are unknown (e.g. imported)
// and treated as "always ok" by the caller.
func collectSignatureExceptionFlags(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd.Name == nil {
			continue
		}
		out[sd.Name.String()] = sd.Exception != nil
	}
	return out
}
