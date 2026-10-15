// port_signature_rules.go enforces ETSI ES 201 873-1 clauses
// 22.3.x: signatures used in procedure-port operations (`call`,
// `reply`, `raise`, `getcall`, `getreply`, `catch`) must be in
// the port type's `in` / `out` / `inout` signature list.
//
// The check is intentionally narrow: it only fires for a bare
// `port.OP(SigName : ...)` expression where:
//
//   - the enclosing function declares a `runs on` component;
//   - the component lists `port <Type> <port>` directly;
//   - the port type's attributes list the allowed signatures by
//     identifier (no parametric expressions);
//   - the call's first argument's left-hand identifier is a known
//     signature name.
//
// Anything outside this conservative envelope is left alone.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPortSignatureRules(mod *syntax.Module) []Diagnostic {
	signatures := collectSignatureNames(mod)
	if len(signatures) == 0 {
		return nil
	}
	portSigs := collectPortTypeSignatures(mod, signatures)
	if len(portSigs) == 0 {
		return nil
	}
	componentPorts := collectComponentPorts(mod)
	if len(componentPorts) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		runsOn := runsOnComponentName(fn.RunsOn)
		ports := componentPorts[runsOn]
		if len(ports) == 0 {
			continue
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil {
				return true
			}
			sel, ok := ce.Fun.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			port := identName(sel.X)
			op := identName(sel.Sel)
			if port == "" || !isProcedureOp(op) {
				return true
			}
			portType, ok := ports[port]
			if !ok {
				return true
			}
			allowed := portSigs[portType]
			if allowed == nil {
				return true
			}
			if ce.Args == nil || len(ce.Args.List) == 0 {
				return true
			}
			sigName := signatureNameOfArg(ce.Args.List[0])
			if sigName == "" {
				// `raise` and `catch` take the signature
				// as a bare ident rather than as the LHS
				// of a `Sig:{}` template instance.
				if op == "raise" || op == "catch" {
					sigName = identName(ce.Args.List[0])
				}
			}
			if sigName == "" {
				return true
			}
			if !signatures[sigName] {
				return true
			}
			if allowed[sigName] {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "port-op-signature-not-listed",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s.%s uses signature %q which is not in port type %q's in/out/inout list (ETSI 22.3)",
					port, op, sigName, portType),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}
	return diags
}

func isProcedureOp(op string) bool {
	switch op {
	case "call", "reply", "raise", "getcall", "getreply", "catch":
		return true
	}
	return false
}

// signatureNameOfArg returns the signature identifier on the LHS of
// a template instance argument such as `SigName:{...}` or
// `SigName:?`. Returns "" for any other shape.
func signatureNameOfArg(x syntax.Expr) string {
	be, ok := x.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ""
	}
	return identName(be.X)
}

// collectSignatureNames returns the set of `signature S(...)`
// declarations in the module.
func collectSignatureNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		sd, ok := d.Def.(*syntax.SignatureDecl)
		if !ok || sd == nil || sd.Name == nil {
			continue
		}
		out[sd.Name.String()] = true
	}
	return out
}

// collectPortTypeSignatures returns a map from port-type name to
// the set of signatures it lists under any `in` / `out` / `inout`
// attribute. Anything that isn't a bare ident in the signature list
// (e.g. parametric port templates) is silently dropped.
func collectPortTypeSignatures(mod *syntax.Module, signatures map[string]bool) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		pt, ok := d.Def.(*syntax.PortTypeDecl)
		if !ok || pt == nil || pt.Name == nil {
			continue
		}
		set := map[string]bool{}
		for _, attr := range pt.Attrs {
			pa, ok := attr.(*syntax.PortAttribute)
			if !ok || pa == nil {
				continue
			}
			switch pa.KindTok.Kind() {
			case syntax.IN, syntax.OUT, syntax.INOUT:
				for _, t := range pa.Types {
					if n := identName(t); n != "" && signatures[n] {
						set[n] = true
					}
				}
			}
		}
		if len(set) > 0 {
			out[pt.Name.String()] = set
		}
	}
	return out
}

