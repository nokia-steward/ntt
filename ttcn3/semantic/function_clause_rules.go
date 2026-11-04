// function_clause_rules.go enforces ETSI ES 201 873-1 clause
// 16.1.0 on the `system` clause attached to a function or altstep:
//
//   - the type a callee's `system` clause references shall be
//     system-compatible (6.3.3 item 4) with the type of the system
//     component. Where the caller knows that type - its own
//     `system` clause, or a testcase's `runs on` clause when it has
//     no `system` clause - a callee whose system type has a port
//     definition missing from it, or declared differently, is
//     rejected (call-system-incompatible).
//
// A `system` clause needs no `runs on` clause, and a caller that
// does not know the system type may call such a function: a wrong
// system is then a runtime error (6.3.3 item 4). Calls from the
// control part are checked by controlCallsWithMtcOrSystem.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkFunctionClauseRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	funcs := collectFuncDeclsByName(mod)
	parents := collectComponentParents(mod)
	defs := collectComponentDefs(mod, parents)

	var diags []Diagnostic
	for _, fd := range funcs {
		system := knownSystemType(fd)
		if system == "" || fd.Body == nil {
			continue
		}
		syntax.Inspect(fd.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok {
				return true
			}
			id, ok := ce.Fun.(*syntax.Ident)
			if !ok {
				return true
			}
			callee := funcs[id.String()]
			if callee == nil || callee.System == nil || callee.KindTok == nil ||
				callee.KindTok.Kind() == syntax.TESTCASE {
				return true
			}
			want := syntax.Name(callee.System.Comp)
			if systemCompatible(system, want, parents, defs) {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "call-system-incompatible",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q has system clause %s, which is not system-compatible with the system type %s (ETSI 16.1.0, 6.3.3)",
					id.String(), want, system),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}
	return diags
}

// knownSystemType is the system component type the body of fd can
// rely on: its `system` clause, or for a testcase without one its
// `runs on` clause (6.3.3 item 4). "" means unknown.
func knownSystemType(fd *syntax.FuncDecl) string {
	if fd.System != nil {
		return syntax.Name(fd.System.Comp)
	}
	if fd.KindTok != nil && fd.KindTok.Kind() == syntax.TESTCASE &&
		fd.RunsOn != nil {
		return syntax.Name(fd.RunsOn.Comp)
	}
	return ""
}

// systemCompatible reports whether a behaviour whose system clause
// names `want` may run where the system is of type `have`: every port
// definition of `want` has an identical definition in `have` (6.3.3
// item 4). Component types we cannot see are taken as compatible.
func systemCompatible(have, want string, parents map[string][]string, defs map[string]map[string]string) bool {
	if have == "" || want == "" || extendsOrEqual(have, want, parents) {
		return true
	}
	haveDefs, ok := defs[have]
	if !ok {
		return true
	}
	wantDefs, ok := defs[want]
	if !ok {
		return true
	}
	for name, def := range wantDefs {
		if isPortDef(def) && haveDefs[name] != def {
			return false
		}
	}
	return true
}
