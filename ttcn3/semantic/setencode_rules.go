// setencode_rules.go enforces ETSI ES 201 873-1 clause 27.9
// "Dynamic configuration" restrictions on the `setencode`
// port operation:
//
//   - The first parameter must reference a type name (or a field
//     of a type), not a template / template variable.
//   - The SingleExpression used in the second parameter must be
//     compatible with the `universal charstring` family (i.e. the
//     `charstring` / `universal charstring` literals or
//     identifiers of those types).
//
// We deliberately keep the rule conservative: only obvious type
// errors trip it. The port-type / nested-field membership check
// (restriction a) is left to a later pass that has a real type
// resolver wired in.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSetencodeRules(mod *syntax.Module) []Diagnostic {
	templateNames := collectTemplateNames(mod)
	varTypes := collectModuleVarTypes(mod)
	portTypes := collectPortMessageTypes(mod)
	declaredTypes := collectDeclaredTypeNames(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Args == nil {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok || op == nil || op.String() != "setencode" {
			return true
		}
		args := ce.Args.List
		if len(args) >= 1 {
			if id, ok := args[0].(*syntax.Ident); ok && id != nil {
				if templateNames[id.String()] {
					diags = append(diags, Diagnostic{
						Code:     "setencode-template-arg",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"setencode: first parameter %q is a template; expected a type reference (ETSI 27.9)",
							id.String()),
						Node: args[0],
						Span: syntax.SpanOf(args[0]),
					})
				}
			}
			// Restriction a: the referenced type (or the type whose
			// field is referenced) must be listed in a port
			// definition. Conservative: only types declared in this
			// module are checked, so imported types never trip it.
			if base := setencodeBaseTypeName(args[0]); base != "" &&
				declaredTypes[base] && !portTypes[base] {
				diags = append(diags, Diagnostic{
					Code:     "setencode-type-not-in-port",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"setencode: type %q is not listed in any port definition (ETSI 27.9 a)",
						base),
					Node: args[0],
					Span: syntax.SpanOf(args[0]),
				})
			}
		}
		if len(args) >= 2 {
			ty := classifyExecuteArg(args[1], varTypes)
			if ty != "" && ty != "-" &&
				ty != "charstring" && ty != "universal" &&
				ty != "universal charstring" {
				diags = append(diags, Diagnostic{
					Code:     "setencode-encoding-arg-not-charstring",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"setencode: encoding argument must be a (universal) charstring (got %q) (ETSI 27.9)",
						ty),
					Node: args[1],
					Span: syntax.SpanOf(args[1]),
				})
			}
		}
		return true
	})
	return diags
}

// setencodeBaseTypeName extracts the type name a setencode first
// argument references: the identifier itself, or the head of a
// field reference (`MyPDU.field1` -> "MyPDU").
func setencodeBaseTypeName(e syntax.Expr) string {
	switch x := e.(type) {
	case *syntax.Ident:
		return x.String()
	case *syntax.SelectorExpr:
		return setencodeBaseTypeName(x.X)
	}
	return ""
}

// collectPortMessageTypes returns the set of type names listed in
// the in/out/inout clauses of every port definition in the module.
func collectPortMessageTypes(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		pa, ok := n.(*syntax.PortAttribute)
		if !ok || pa == nil {
			return true
		}
		for _, t := range pa.Types {
			if id, ok := t.(*syntax.Ident); ok && id != nil {
				out[id.String()] = true
			}
		}
		return true
	})
	return out
}

// collectDeclaredTypeNames returns the names of every type declared
// in the module, so membership checks can skip imported types.
func collectDeclaredTypeNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch d := n.(type) {
		case *syntax.StructTypeDecl:
			if d.Name != nil {
				out[syntax.Name(d.Name)] = true
			}
		case *syntax.SubTypeDecl:
			if d.Field != nil && d.Field.Name != nil {
				out[syntax.Name(d.Field.Name)] = true
			}
		case *syntax.EnumTypeDecl:
			if d.Name != nil {
				out[syntax.Name(d.Name)] = true
			}
		}
		return true
	})
	return out
}

// collectTemplateNames returns the set of every named template
// declaration (module-level or local). The lookup is used to tell
// templates apart from type identifiers in setencode's first arg.
func collectTemplateNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		out[td.Name.String()] = true
		return true
	})
	return out
}
