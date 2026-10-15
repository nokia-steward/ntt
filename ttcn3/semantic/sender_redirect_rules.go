// sender_redirect_rules.go enforces ETSI ES 201 873-1 clause 22
// rules on `-> sender X` redirects of port operations:
//
//   - With no `from` clause: X's declared type must match the
//     enclosing function's runs-on component type.
//
//   - With a `from CompName:?` clause: X's declared type must match
//     CompName.
//
// The check is intentionally narrow: it only fires when both sides
// are known component-type names declared in the same module and
// no `extends` relationship hides the mismatch. Address-typedef
// senders, signatures, and component-extension chains stay silent.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSenderRedirectRules(mod *syntax.Module) []Diagnostic {
	components := collectComponentTypeNames(mod)
	if len(components) == 0 {
		return nil
	}
	varTypes := collectModuleVarTypes(mod)
	primAliases := collectPrimitiveTypeAliases(mod)
	addressVars := collectPortAddressVars(mod)
	hasAddress := moduleHasAddressType(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch v := d.Def.(type) {
		case *syntax.FuncDecl:
			diags = append(diags, walkFuncForSenderRedirects(v, varTypes, primAliases, addressVars, components, hasAddress)...)
		}
	}
	return diags
}

// moduleHasAddressType reports whether the module declares an
// address type (`type T address;`). When true, ports in this module
// may carry address values - so we cannot insist that a sender
// redirect variable be a component reference: it might legitimately
// be an address-typed value (ETSI 22.2.2).
func moduleHasAddressType(mod *syntax.Module) bool {
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		td, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || td == nil || td.Field == nil || td.Field.Name == nil {
			continue
		}
		if td.Field.Name.String() == "address" {
			return true
		}
	}
	return false
}

func collectPortAddressVars(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || !isPortAddressTypeExpr(vd.Type) {
			return true
		}
		for _, d := range vd.Decls {
			if d != nil && d.Name != nil {
				out[d.Name.String()] = true
			}
		}
		return true
	})
	return out
}

func isPortAddressTypeExpr(e syntax.Expr) bool {
	sel, ok := e.(*syntax.SelectorExpr)
	if !ok || sel == nil {
		return false
	}
	return identName(sel.X) != "" && identName(sel.Sel) == "address"
}

func collectConnectedPortNames(body *syntax.BlockStmt) map[string]bool {
	out := map[string]bool{}
	if body == nil {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil {
			return true
		}
		if identName(ce.Fun) != "connect" {
			return true
		}
		for _, arg := range ce.Args.List {
			if port := endpointPortName(arg); port != "" {
				out[port] = true
			}
		}
		return true
	})
	return out
}

func endpointPortName(e syntax.Expr) string {
	be, ok := e.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ""
	}
	return identName(be.Y)
}

func redirectedPortName(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.SelectorExpr:
		return identName(x.X)
	case *syntax.CallExpr:
		if sel, ok := x.Fun.(*syntax.SelectorExpr); ok && sel != nil {
			return identName(sel.X)
		}
	}
	return ""
}

func walkFuncForSenderRedirects(
	fn *syntax.FuncDecl,
	varTypes map[string]string,
	primAliases map[string]string,
	addressVars map[string]bool,
	components map[string]bool,
	hasAddress bool,
) []Diagnostic {
	if fn == nil || fn.Body == nil {
		return nil
	}
	runsOn := runsOnComponentName(fn.RunsOn)
	connectedPorts := collectConnectedPortNames(fn.Body)
	var diags []Diagnostic
	syntax.Inspect(fn.Body, func(n syntax.Node) bool {
		// Look for the two redirect shapes that the parser
		// produces for `<receive> [from C:?] -> sender X`:
		//
		//   - bare:    RedirectExpr.Sender = X
		//   - from-C:  BinaryExpr{Op=from, Y=BinaryExpr{Op=:,
		//                X=Ident("C"), Y=RedirectExpr.Sender=X}}
		//
		// The from-clause path is detected by walking up from
		// the RedirectExpr; we cheat by recognising the
		// from-clause shape directly when we see it.
		switch be := n.(type) {
		case *syntax.BinaryExpr:
			if be == nil || be.Op == nil || be.Op.Kind() != syntax.FROM {
				return true
			}
			expected := fromClauseComponent(be.Y)
			rx := findInnerRedirect(be.Y)
			if rx == nil || rx.Sender == nil || expected == "" {
				return true
			}
			if !components[expected] {
				return true
			}
			if d := senderMismatchDiag(rx, expected, "from-clause", varTypes, primAliases, components, false); d != nil {
				diags = append(diags, *d)
			}
			return false
		case *syntax.RedirectExpr:
			if be == nil || be.Sender == nil {
				return true
			}
			op := redirectOpName(be.X)
			if !isPortOperationName(op) {
				return true
			}
			if target := identName(be.Sender); target != "" && addressVars[target] {
				if port := redirectedPortName(be.X); port != "" && connectedPorts[port] {
					diags = append(diags, Diagnostic{
						Code:     "sender-address-on-connected-port",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"sender redirect target %q has port-address type, but %s.%s receives on a connected component port (ETSI 22.2)",
							target, port, op),
						Node: be,
						Span: syntax.SpanOf(be),
					})
					return true
				}
			}
			if runsOn == "" || !components[runsOn] {
				return true
			}
			if d := senderMismatchDiag(be, runsOn, "runs-on", varTypes, primAliases, components, hasAddress); d != nil {
				diags = append(diags, *d)
			}
			return true
		}
		return true
	})
	return diags
}

// senderMismatchDiag returns a diagnostic when the sender ident's
// declared type cannot match `expected`. Two sub-rules fire:
//
//   - The sender's type is a known component name (declared in the
//     same module) and differs from `expected`.
//
//   - `expected` is a component and the sender's type is a known
//     non-component type-alias (e.g. `type integer address`). The
//     sender cannot bind a component value into a non-component
//     variable.
//
// Returns nil when the sender's type can't be determined, the
// types match, or only one side is recognised as a component.
func senderMismatchDiag(r *syntax.RedirectExpr, expected, ctx string, varTypes map[string]string, primAliases map[string]string, components map[string]bool, skipNonComponentRule bool) *Diagnostic {
	target := identName(r.Sender)
	if target == "" {
		return nil
	}
	ty, ok := varTypes[target]
	if !ok || ty == "" {
		return nil
	}
	if components[ty] {
		if ty == expected {
			return nil
		}
		return &Diagnostic{
			Code:     "sender-redirect-component-mismatch",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"sender redirect target %q has component type %q; %s expects %q (ETSI 22)",
				target, ty, ctx, expected),
			Node: r,
			Span: syntax.SpanOf(r),
		}
	}
	// Sender is a non-component declared type while the
	// expected sender is a component. Only fire when ty is a
	// recognised primitive or a typedef that ultimately
	// resolves to a primitive - those can never hold a
	// component reference. `skipNonComponentRule` suppresses
	// the diagnostic when the module declares an `address`
	// type (in which case a non-component sender variable may
	// be holding an address value rather than a component
	// reference).
	if skipNonComponentRule {
		return nil
	}
	if isPrimitiveType(ty) || isPrimitiveType(primAliases[ty]) {
		return &Diagnostic{
			Code:     "sender-redirect-non-component",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"sender redirect target %q has type %q; %s expects component %q (ETSI 22)",
				target, ty, ctx, expected),
			Node: r,
			Span: syntax.SpanOf(r),
		}
	}
	return nil
}

// collectPrimitiveTypeAliases returns a typedef-name -> primitive
// type-name map for any `type <Primitive> <Alias>` declaration. The
// map is closed transitively, so `type address integer` and
// `type address ShortHand` both resolve to "integer".
func collectPrimitiveTypeAliases(mod *syntax.Module) map[string]string {
	direct := map[string]string{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		td, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || td == nil || td.Field == nil || td.Field.Name == nil {
			continue
		}
		ref, ok := td.Field.Type.(*syntax.RefSpec)
		if !ok || ref == nil {
			continue
		}
		ty := identName(ref.X)
		if ty == "" {
			continue
		}
		direct[td.Field.Name.String()] = ty
	}
	out := map[string]string{}
	for alias := range direct {
		seen := map[string]bool{}
		cur := alias
		for {
			next, ok := direct[cur]
			if !ok {
				if isPrimitiveType(cur) {
					out[alias] = cur
				}
				break
			}
			if seen[next] {
				break
			}
			seen[next] = true
			cur = next
		}
	}
	return out
}

// fromClauseComponent returns the component-name ident referenced
// by the X of a from-clause shape `Ident("C") : <whatever>` (the
// `:?` template-instance part). Returns "" for any other shape.
func fromClauseComponent(x syntax.Expr) string {
	be, ok := x.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.COLON {
		return ""
	}
	return identName(be.X)
}

// findInnerRedirect walks an arbitrary expression shape looking for
// the inner RedirectExpr produced by a from-clause Binary tree.
// We descend through BinaryExpr only - other shapes terminate the
// walk because the parser never wraps the redirect in anything
// else here.
func findInnerRedirect(x syntax.Expr) *syntax.RedirectExpr {
	for {
		switch v := x.(type) {
		case *syntax.RedirectExpr:
			return v
		case *syntax.BinaryExpr:
			if v == nil {
				return nil
			}
			x = v.Y
		default:
			return nil
		}
	}
}

// runsOnComponentName returns the component-type identifier of a
// RunsOnSpec, or "" if there is none / the spec isn't a bare ident.
func runsOnComponentName(rs *syntax.RunsOnSpec) string {
	if rs == nil {
		return ""
	}
	return identName(rs.Comp)
}
