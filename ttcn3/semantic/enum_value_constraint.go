// enum_value_constraint.go enforces ETSI ES 201 873-1 clause
// 6.1.2.1 (value-list subtype) for boolean and verdicttype, the
// two enumerated-flavour base types whose value space is finite
// enough to validate statically from literal RHS expressions.
//
// The numeric-range validator in value_constraint.go intentionally
// limits itself to integer / float so the existing range / list
// resolver doesn't have to model boolean truth tables or the
// verdicttype lattice. This file adds the small bool / verdict
// case independently:
//
//   - collectEnumConstraints collects every named subtype of
//     boolean / verdicttype with a value-list constraint, including
//     the 6.1.2.2 list-of-types form `type T (T1, T2);` via the
//     same fixpoint shape value_constraint.go uses.
//   - checkEnumValueRules walks function / testcase / altstep
//     bodies and emits enum-value-out-of-list when:
//   - a `var T v := lit;` initializer assigns a literal not in T's
//     constraint set, or
//   - a `v := lit;` assignment to an existing T-typed local does
//     the same.
//
// The check is conservative: any RHS that isn't a recognisable
// boolean / verdict literal is left to the runtime.
package semantic

import (
	"fmt"
	"sort"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// enumSpec captures the literal-value allow-set for a named
// boolean / verdicttype subtype. base is the underlying family
// name ("boolean" / "verdicttype"), values is the lower-cased set
// of accepted token spellings.
type enumSpec struct {
	base   string
	values map[string]bool
}

// enumKindOfBase returns the family string for an enum-flavour
// base type, or "" for anything else.
func enumKindOfBase(name string) string {
	switch name {
	case "boolean", "verdicttype":
		return name
	}
	return ""
}

// enumLiteralKind returns the family ("boolean" / "verdicttype")
// and the spelling of a literal that is recognisable as such.
// Returns ("", "") for any other expression shape.
func enumLiteralKind(e syntax.Expr) (string, string) {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return "", ""
	}
	switch lit.Tok.Kind() {
	case syntax.TRUE:
		return "boolean", "true"
	case syntax.FALSE:
		return "boolean", "false"
	case syntax.PASS:
		return "verdicttype", "pass"
	case syntax.FAIL:
		return "verdicttype", "fail"
	case syntax.INCONC:
		return "verdicttype", "inconc"
	case syntax.NONE:
		return "verdicttype", "none"
	case syntax.ERROR:
		return "verdicttype", "error"
	}
	return "", ""
}

// collectEnumConstraints walks every top-level SubTypeDecl whose
// base is boolean / verdicttype and gathers the literal-value
// allow-set into an enumSpec. The 6.1.2.2 list-of-types form
// (`type T (T1, T2);`) is resolved via a small fixpoint that mirrors
// collectValueConstrainedTypes.
func collectEnumConstraints(mod *syntax.Module) map[string]enumSpec {
	out := map[string]enumSpec{}
	// rawRefs tracks subtypes whose constraint pulls in other
	// named subtypes that may not be resolved yet.
	rawRefs := map[string][]string{}
	rawBase := map[string]string{}
	rawPartial := map[string]map[string]bool{}

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st.Field == nil || st.Field.Name == nil ||
			st.Field.ValueConstraint == nil {
			continue
		}
		base := enumBaseOf(st.Field.Type)
		if base == "" {
			continue
		}
		vals, refs := parseEnumConstraint(st.Field.ValueConstraint, base)
		name := st.Field.Name.String()
		if len(refs) == 0 {
			out[name] = enumSpec{base: base, values: vals}
			continue
		}
		rawPartial[name] = vals
		rawRefs[name] = refs
		rawBase[name] = base
	}

	for changed := true; changed; {
		changed = false
		for name, refs := range rawRefs {
			if _, done := out[name]; done {
				continue
			}
			merged := rawPartial[name]
			if merged == nil {
				merged = map[string]bool{}
			}
			allResolved := true
			for _, refName := range refs {
				ref, ok := out[refName]
				if !ok {
					allResolved = false
					break
				}
				for v := range ref.values {
					merged[v] = true
				}
			}
			if !allResolved {
				continue
			}
			out[name] = enumSpec{base: rawBase[name], values: merged}
			delete(rawRefs, name)
			changed = true
		}
	}
	return out
}

// enumBaseOf reports the boolean / verdicttype base of a Field
// TypeSpec, or "" otherwise.
func enumBaseOf(spec syntax.TypeSpec) string {
	ref, ok := spec.(*syntax.RefSpec)
	if !ok || ref == nil || ref.X == nil {
		return ""
	}
	id, ok := ref.X.(*syntax.Ident)
	if !ok || id == nil {
		return ""
	}
	return enumKindOfBase(id.String())
}

// parseEnumConstraint collapses a ParenExpr value-list into the
// recognised literal spellings plus a list of Ident references
// for the 6.1.2.2 list-of-types form.
func parseEnumConstraint(
	pe *syntax.ParenExpr,
	base string,
) (map[string]bool, []string) {
	out := map[string]bool{}
	var refs []string
	for _, item := range pe.List {
		if id, ok := item.(*syntax.Ident); ok && id != nil {
			refs = append(refs, id.String())
			continue
		}
		if fam, name := enumLiteralKind(item); fam == base {
			out[name] = true
		}
	}
	return out, refs
}

// checkEnumValueRules is wired into Analyze; emits the
// enum-value-out-of-list diagnostics.
func (a *Analyzer) checkEnumValueRules(mod *syntax.Module) []Diagnostic {
	tab := collectEnumConstraints(mod)
	if len(tab) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			return true
		}
		vars := collectLocalEnumVars(fn.Body, tab)
		syntax.Inspect(fn.Body, func(sn syntax.Node) bool {
			switch x := sn.(type) {
			case *syntax.ValueDecl:
				diags = append(diags, checkEnumValueDeclInit(x, tab)...)
			case *syntax.BinaryExpr:
				if x.Op != nil && x.Op.Kind() == syntax.ASSIGN {
					diags = append(diags, checkEnumAssignment(x, vars, tab)...)
				}
			}
			return true
		})
		return false
	})
	return diags
}

// collectLocalEnumVars maps each local variable declared in body
// to its enum-constrained type name (or "" if not enum-constrained).
func collectLocalEnumVars(body syntax.Node, tab map[string]enumSpec) map[string]string {
	out := map[string]string{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := identName(vd.Type)
		if typeName == "" {
			return true
		}
		if _, isEnum := tab[typeName]; !isEnum {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = typeName
		}
		return true
	})
	return out
}

// checkEnumValueDeclInit validates the `var T v := lit;` shape.
func checkEnumValueDeclInit(vd *syntax.ValueDecl, tab map[string]enumSpec) []Diagnostic {
	typeName := identName(vd.Type)
	spec, ok := tab[typeName]
	if !ok {
		return nil
	}
	var diags []Diagnostic
	for _, dec := range vd.Decls {
		if dec == nil || dec.Value == nil {
			continue
		}
		if d := checkEnumLiteralValue(dec.Value, spec, typeName); d != nil {
			diags = append(diags, *d)
		}
	}
	return diags
}

// checkEnumAssignment validates `v := lit;` where v is a local
// enum-constrained variable.
func checkEnumAssignment(
	be *syntax.BinaryExpr,
	vars map[string]string,
	tab map[string]enumSpec,
) []Diagnostic {
	varName := identName(be.X)
	if varName == "" {
		return nil
	}
	typeName, isEnum := vars[varName]
	if !isEnum {
		return nil
	}
	spec, ok := tab[typeName]
	if !ok {
		return nil
	}
	if d := checkEnumLiteralValue(be.Y, spec, typeName); d != nil {
		return []Diagnostic{*d}
	}
	return nil
}

// checkEnumLiteralValue returns a diagnostic when val is a literal
// of the right family but a value not in spec.values.
func checkEnumLiteralValue(val syntax.Expr, spec enumSpec, typeName string) *Diagnostic {
	fam, name := enumLiteralKind(val)
	if fam == "" || fam != spec.base {
		return nil
	}
	if spec.values[name] {
		return nil
	}
	allowed := make([]string, 0, len(spec.values))
	for v := range spec.values {
		allowed = append(allowed, v)
	}
	sort.Strings(allowed)
	return &Diagnostic{
		Code:     "enum-value-out-of-list",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"value %s is not in the value list of subtype %q (allowed: %v)",
			name, typeName, allowed),
		Node: val,
		Span: syntax.SpanOf(val),
	}
}
