// anytype_selector_rules.go enforces the ETSI ES 201 873-1
// clause 6.2.0.6 / 6.2.6 restrictions on `anytype` selectors.
//
// The anytype is "the union of all known data types in the
// module". Selecting a field of an anytype variable via
// `var.TypeName` is therefore only legal when TypeName is one of
// the known data types of the module:
//
//   - the built-in basic types (integer, float, boolean, the
//     string family, verdicttype, anytype);
//   - the module's user-declared data types (subtypes, structured
//     types, enums, unions, classes, map types).
//
// Port types, component types, behaviour types (function /
// altstep / testcase types), default types, timers and the
// special `address` keyword are *not* known data types and using
// them as anytype selectors is forbidden. The default- / timer-
// based subtypes (`type default Mydef;`, `type timer MyTimer;`)
// inherit that ban.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAnytypeSelectorRules(mod *syntax.Module) []Diagnostic {
	anytypeVars := collectAnytypeVarNames(mod)
	if len(anytypeVars) == 0 {
		return nil
	}
	info := collectModuleTypeKinds(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base == nil {
			return true
		}
		if !anytypeVars[base.String()] {
			return true
		}
		field, ok := sel.Sel.(*syntax.Ident)
		if !ok || field == nil {
			return true
		}
		name := field.String()
		switch reason := anytypeFieldRejection(name, info); reason {
		case "":
			// allowed
		default:
			diags = append(diags, Diagnostic{
				Code:     "anytype-selector-forbidden",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"anytype variable %q cannot select %q: %s (ETSI 6.2.6)",
					base.String(), name, reason),
				Node: sel,
				Span: syntax.SpanOf(sel),
			})
		}
		return true
	})
	return diags
}

// collectAnytypeVarNames returns the set of variable identifiers
// declared as `var anytype X` in the module (locals + component
// var members). Templates and constants are excluded.
func collectAnytypeVarNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if identName(vd.Type) != "anytype" {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			out[d.Name.String()] = true
		}
		return true
	})
	return out
}

// moduleTypeKinds catalogues every user-declared type so the
// anytype-selector check can classify a referenced TypeName.
type moduleTypeKinds struct {
	// dataTypes is every type that the anytype can legally
	// embed: subtypes (whose base is a data type or another
	// data type), struct types, enums, unions, classes, maps.
	dataTypes map[string]bool
	// portTypes records names declared with `type port`.
	portTypes map[string]bool
	// componentTypes records `type component` declarations.
	componentTypes map[string]bool
	// behaviourTypes records `type testcase/function/altstep`.
	behaviourTypes map[string]bool
	// defaultBased records subtypes derived from the `default`
	// built-in (e.g. `type default Mydef;`).
	defaultBased map[string]bool
	// timerBased records subtypes derived from `timer`.
	timerBased map[string]bool
	// addressBased records subtypes derived from `address`.
	addressBased map[string]bool
}

func collectModuleTypeKinds(mod *syntax.Module) moduleTypeKinds {
	out := moduleTypeKinds{
		dataTypes:      map[string]bool{},
		portTypes:      map[string]bool{},
		componentTypes: map[string]bool{},
		behaviourTypes: map[string]bool{},
		defaultBased:   map[string]bool{},
		timerBased:     map[string]bool{},
		addressBased:   map[string]bool{},
	}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch v := d.Def.(type) {
		case *syntax.SubTypeDecl:
			if v == nil || v.Field == nil || v.Field.Name == nil {
				continue
			}
			name := v.Field.Name.String()
			baseName := ""
			if rs, ok := v.Field.Type.(*syntax.RefSpec); ok && rs != nil {
				baseName = identName(rs.X)
			}
			switch baseName {
			case "default":
				out.defaultBased[name] = true
			case "timer":
				out.timerBased[name] = true
			default:
				out.dataTypes[name] = true
			}
			// `type integer address;` makes `address`
			// a known data type for the module; the
			// anytype-selector check needs this even
			// though baseName is "integer".
			if name == "address" {
				out.addressBased[name] = true
				out.dataTypes[name] = true
			}
		case *syntax.StructTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.dataTypes[v.Name.String()] = true
		case *syntax.EnumTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.dataTypes[v.Name.String()] = true
		case *syntax.MapTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.dataTypes[v.Name.String()] = true
		case *syntax.ClassTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.dataTypes[v.Name.String()] = true
		case *syntax.PortTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.portTypes[v.Name.String()] = true
		case *syntax.ComponentTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.componentTypes[v.Name.String()] = true
		case *syntax.BehaviourTypeDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out.behaviourTypes[v.Name.String()] = true
		}
	}
	return out
}

// anytypeFieldRejection returns a human-readable reason describing
// why `name` cannot appear as an anytype field selector, or ""
// when the selector is legal.
func anytypeFieldRejection(name string, info moduleTypeKinds) string {
	if isBuiltinAnytypeMember(name) {
		return ""
	}
	if info.portTypes[name] {
		return "port type"
	}
	if info.componentTypes[name] {
		return "component type"
	}
	if info.behaviourTypes[name] {
		return "behaviour (testcase/function/altstep) type"
	}
	if info.defaultBased[name] {
		return "default-based subtype"
	}
	if info.timerBased[name] {
		return "timer-based subtype"
	}
	if name == "address" && !info.addressBased[name] {
		return "address type is not declared in the module"
	}
	if name == "default" || name == "timer" {
		return name + " is not a data type"
	}
	if name == "port" || name == "component" {
		return name + " is not a data type"
	}
	if info.dataTypes[name] {
		return ""
	}
	// Unknown selectors fall through silently - the selector
	// could refer to a built-in or imported type we don't model
	// explicitly; we err on the side of accepting them.
	return ""
}

func isBuiltinAnytypeMember(name string) bool {
	switch name {
	case "integer", "float", "boolean", "verdicttype",
		"charstring", "bitstring", "hexstring", "octetstring":
		return true
	case "anytype":
		// anytype can legally nest - the inner anytype
		// resolves to the same union of known types.
		return true
	}
	return false
}
