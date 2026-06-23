package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkTimerReceiverRules enforces ETSI 23.{2-6}: the receiver of
// `start`/`stop`/`read`/`running`/`timeout` MUST be a timer (or
// the `any timer` / `all timer` qualifier). Calling those on a
// `float` / `integer` / etc. value reference is a static error.
//
// We only flag when we can name the receiver with high
// confidence as a non-timer:
//
//   - Variables declared with `var T x` where T is a non-timer
//     primitive type identifier.
//   - Variables declared with `var T x` where T resolves to a
//     known non-timer type alias (we don't follow aliases - we
//     just check the literal type ident).
//   - Function parameters of non-timer type.
//
// If we can't classify the receiver, we say nothing. This keeps
// false positives at zero on the conformance suite.
func (a *Analyzer) checkTimerReceiverRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	timers := collectModuleTimerIdents(mod)
	nonTimers := collectModuleNonTimerIdents(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		opIdent, ok := sel.Sel.(*syntax.Ident)
		if !ok || opIdent == nil {
			return true
		}
		op := opIdent.String()
		if !timerOp(op) {
			return true
		}
		recvIdent, ok := sel.X.(*syntax.Ident)
		if !ok || recvIdent == nil {
			return true
		}
		name := recvIdent.String()
		if timers[name] {
			return true
		}
		if !nonTimers[name] {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "timer-op-non-timer-receiver",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`%s.%s` requires a timer receiver; `%s` is not declared as timer (ETSI 23.{2-6})",
				name, op, name),
			Node: sel,
			Span: syntax.SpanOf(sel),
		})
		return true
	})
	return diags
}

// collectModuleNonTimerIdents walks the module for value
// declarations whose type ident is a known TTCN-3 primitive that
// is NOT `timer`. Conservative: ignores aliases, component refs,
// and complex shape expressions, so we only flag obvious misuses.
func collectModuleNonTimerIdents(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	primitive := map[string]bool{
		"integer":   true,
		"float":     true,
		"boolean":   true,
		"charstring": true,
		"universal":  true,
		"bitstring":  true,
		"hexstring":  true,
		"octetstring": true,
		"verdicttype": true,
		"anytype":     true,
		"default":     true,
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.FormalPar:
			if v == nil || v.Name == nil || v.Type == nil {
				return true
			}
			if id, ok := v.Type.(*syntax.Ident); ok && id != nil && id.Tok != nil {
				if primitive[id.Tok.String()] {
					out[v.Name.String()] = true
				}
			}
		case *syntax.ValueDecl:
			if v == nil {
				return true
			}
			if v.Type != nil {
				if id, ok := v.Type.(*syntax.Ident); ok && id != nil && id.Tok != nil {
					if primitive[id.Tok.String()] {
						for _, d := range v.Decls {
							if d == nil || d.Name == nil {
								continue
							}
							out[d.Name.String()] = true
						}
					}
				}
			}
		}
		return true
	})
	return out
}
