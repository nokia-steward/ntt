package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkMapUnmapParamRules enforces ETSI 21.1.{1,2} restriction b:
// if a `map`/`unmap`/`connect`/`disconnect` operation carries a
// `param(...)` clause, the actual arguments must conform to the
// `map param (...)` / `unmap param (...)` clause declared on the
// system port's type:
//
//   - If the port type DOES NOT declare a matching `<op> param`
//     clause, the operation must not carry `param(...)`.
//   - If it does, the actual count must match the declared count.
//
// We do NOT do per-argument type matching here; the easy cohort
// wins come from "no clause at all" and "wrong arity" detection.
func (a *Analyzer) checkMapUnmapParamRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	portMapParams := collectPortMapParams(mod)
	if len(portMapParams) == 0 {
		return diags
	}
	portTypeOfVar := collectPortVarTypes(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		pe, ok := n.(*syntax.ParamExpr)
		if !ok || pe == nil || pe.X == nil || pe.Y == nil {
			return true
		}
		ce, ok := pe.X.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Fun == nil {
			return true
		}
		opIdent, ok := ce.Fun.(*syntax.Ident)
		if !ok || opIdent == nil {
			return true
		}
		op := opIdent.String()
		switch op {
		case "map", "unmap", "connect", "disconnect":
		default:
			return true
		}
		// Restriction a (ETSI 21.1.{1,2}): a `param(...)` clause on a
		// `map`/`unmap` may only be present when the system port it
		// belongs to is explicitly referenced (`system:p`). Using it
		// with only component-side endpoints (`unmap(self:p) param(...)`)
		// is illegal.
		if (op == "map" || op == "unmap") && !hasSystemPortArg(ce) {
			diags = append(diags, Diagnostic{
				Code:     "map-param-no-system",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`%s` carries a `param(...)` clause but references no `system` port (ETSI 21.1.{1,2} restriction a)",
					op),
				Node: pe,
				Span: syntax.SpanOf(pe),
			})
			return true
		}
		actualArgs := paramExprArgList(pe)
		// Heuristic: any of the call's positional args is a
		// `<comp>:<port>` BinaryExpr. We pick the FIRST one
		// we can resolve to a known port type. All ports in
		// a single map/unmap call must share a compatible
		// `<op> param` clause anyway.
		portType := ""
		if ce.Args != nil {
			for _, arg := range ce.Args.List {
				if pt := portTypeFromArg(arg, portTypeOfVar); pt != "" {
					portType = pt
					break
				}
			}
		}
		if portType == "" {
			return true
		}
		spec, ok := portMapParams[portType]
		if !ok || spec == nil {
			return true
		}
		formal := spec.byOp(op)
		switch {
		case formal == nil:
			diags = append(diags, Diagnostic{
				Code:     "map-param-unknown",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`%s` carries a `param(...)` clause but port type %q declares no `%s param` (ETSI 21.1.{1,2})",
					op, portType, op),
				Node: pe,
				Span: syntax.SpanOf(pe),
			})
		case formalParamCount(formal) != len(actualArgs):
			diags = append(diags, Diagnostic{
				Code:     "map-param-arity",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"`%s` for port type %q expects %d param argument(s); got %d (ETSI 21.1.{1,2})",
					op, portType, formalParamCount(formal), len(actualArgs)),
				Node: pe,
				Span: syntax.SpanOf(pe),
			})
		default:
			if mismatch := actualParamLiteralTypeMismatch(formal, actualArgs); mismatch != "" {
				diags = append(diags, Diagnostic{
					Code:     "map-param-type",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`%s` for port type %q: %s (ETSI 21.1.{1,2})",
						op, portType, mismatch),
					Node: pe,
					Span: syntax.SpanOf(pe),
				})
			}
		}
		return true
	})
	return diags
}

// portMapParamSet groups the four optional `<op> param` clauses
// declared on a port type. nil entries mean "no clause declared
// for that operation". The struct is keyed by port-type name.
type portMapParamSet struct {
	mapParams        *syntax.FormalPars
	unmapParams      *syntax.FormalPars
	connectParams    *syntax.FormalPars
	disconnectParams *syntax.FormalPars
}

func (s *portMapParamSet) byOp(op string) *syntax.FormalPars {
	if s == nil {
		return nil
	}
	switch op {
	case "map":
		return s.mapParams
	case "unmap":
		return s.unmapParams
	case "connect":
		return s.connectParams
	case "disconnect":
		return s.disconnectParams
	}
	return nil
}

// collectPortMapParams walks the module and indexes every port
// type's `<op> param (...)` clauses by port-type name.
func collectPortMapParams(mod *syntax.Module) map[string]*portMapParamSet {
	out := map[string]*portMapParamSet{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ptd, ok := n.(*syntax.PortTypeDecl)
		if !ok || ptd == nil || ptd.Name == nil {
			return true
		}
		name := ptd.Name.String()
		set := out[name]
		if set == nil {
			set = &portMapParamSet{}
			out[name] = set
		}
		for _, attr := range ptd.Attrs {
			pma, ok := attr.(*syntax.PortMapAttribute)
			if !ok || pma == nil || pma.MapTok == nil {
				continue
			}
			switch pma.MapTok.String() {
			case "map":
				set.mapParams = pma.Params
			case "unmap":
				set.unmapParams = pma.Params
			case "connect":
				set.connectParams = pma.Params
			case "disconnect":
				set.disconnectParams = pma.Params
			}
		}
		return true
	})
	return out
}

// collectPortVarTypes maps port-variable names to their port type
// name. Walks every component body for `port T name;` decls.
func collectPortVarTypes(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil || vd.KindTok.Kind() != syntax.PORT {
			return true
		}
		typeName := ""
		if id, ok := vd.Type.(*syntax.Ident); ok && id != nil {
			typeName = id.String()
		}
		if typeName == "" {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			out[d.Name.String()] = typeName
		}
		return true
	})
	return out
}

// hasSystemPortArg reports whether any argument of a map/unmap call is
// an explicit `system:port` reference.
func hasSystemPortArg(ce *syntax.CallExpr) bool {
	if ce == nil || ce.Args == nil {
		return false
	}
	for _, arg := range ce.Args.List {
		bin, ok := arg.(*syntax.BinaryExpr)
		if !ok || bin == nil {
			continue
		}
		if id, ok := bin.X.(*syntax.Ident); ok && id != nil && id.String() == "system" {
			return true
		}
	}
	return false
}

// portTypeFromArg resolves a single map-arg expression like
// `self:p` or `system:p` or `v_ptc:p` to the port type of the
// port name on the right of the colon. Returns "" if it can't
// figure it out.
func portTypeFromArg(arg syntax.Expr, portTypeOfVar map[string]string) string {
	if arg == nil {
		return ""
	}
	if bin, ok := arg.(*syntax.BinaryExpr); ok && bin != nil && bin.Op != nil && bin.Op.String() == ":" {
		if id, ok := bin.Y.(*syntax.Ident); ok && id != nil {
			return portTypeOfVar[id.String()]
		}
	}
	return ""
}

// paramExprArgList returns the list of actual param arguments
// (e.g. `param(5, "x")` -> [5, "x"]). Handles both ParenExpr and
// bare expression shapes.
func paramExprArgList(pe *syntax.ParamExpr) []syntax.Expr {
	if pe == nil || pe.Y == nil {
		return nil
	}
	if pn, ok := pe.Y.(*syntax.ParenExpr); ok && pn != nil {
		return pn.List
	}
	return []syntax.Expr{pe.Y}
}

// formalParamCount returns the number of non-nil entries in a
// FormalPars list. Defensive against trailing-nil shapes the
// parser may produce on error.
func formalParamCount(fp *syntax.FormalPars) int {
	if fp == nil {
		return 0
	}
	n := 0
	for _, p := range fp.List {
		if p != nil {
			n++
		}
	}
	return n
}

// actualParamLiteralTypeMismatch returns a short description of
// the first observable type mismatch between actuals and the
// declared formal-param types. Mirrors defaultLiteralTypeMismatch
// in parametrization.go but adapted to multi-arg formal lists.
func actualParamLiteralTypeMismatch(fp *syntax.FormalPars, actuals []syntax.Expr) string {
	if fp == nil {
		return ""
	}
	i := 0
	for _, p := range fp.List {
		if p == nil {
			continue
		}
		if i >= len(actuals) {
			break
		}
		actual := actuals[i]
		i++
		lit, ok := actual.(*syntax.ValueLiteral)
		if !ok || lit == nil || lit.Tok == nil {
			continue
		}
		id, ok := p.Type.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			continue
		}
		mismatch := literalKindMismatchFor(id.Tok.String(), lit.Tok.Kind())
		if mismatch != "" {
			name := "<arg>"
			if p.Name != nil {
				name = p.Name.String()
			}
			return fmt.Sprintf(
				"argument %d for %q (%s) is %s",
				i, name, id.Tok.String(), mismatch)
		}
	}
	return ""
}

// literalKindMismatchFor mirrors the switch in
// defaultLiteralTypeMismatch but stays a private helper so map
// param diagnostics can share the table without coupling files.
func literalKindMismatchFor(typeName string, litKind syntax.Kind) string {
	switch typeName {
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
