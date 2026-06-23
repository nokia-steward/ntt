// template_type_rules.go enforces ETSI ES 201 873-1 clause 15
// "Templates" restrictions on template type families:
//
//   - A template's declared type must not be (transitively) of
//     `default`, `port`, or `timer` kind. The standard forbids
//     templates carrying behavioural-state references because
//     they cannot meaningfully be matched against runtime values.
//
// We reuse the port-name catalogue and `nestedForbiddenFamily`
// helper from component_call_rules.go so the recursion through
// record / set / union field-types stays consistent. The rule is
// intentionally type-only - we don't inspect template bodies; the
// declared type alone determines acceptability.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateTypeRules(mod *syntax.Module) []Diagnostic {
	forbiddenParamFamily := map[string]string{}
	for tname := range collectPortTypes(mod) {
		forbiddenParamFamily[tname] = "port"
	}
	// Map types (ETSI 6.2.15.1 restriction a) are forbidden
	// for templates too. We treat them the same way as ports
	// here so the nested walk catches `template T` where T
	// transitively contains a map field.
	for tname := range collectMapSpecs(mod) {
		forbiddenParamFamily[tname] = "map"
	}
	structFieldTypes := collectStructFieldTypes(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Type == nil {
			return true
		}
		typeName := identName(td.Type)
		if typeName == "" {
			return true
		}
		family, bad := nestedForbiddenFamily(typeName,
			forbiddenParamFamily, structFieldTypes, map[string]bool{})
		if !bad {
			return true
		}
		name := ""
		if td.Name != nil {
			name = td.Name.String()
		}
		diags = append(diags, Diagnostic{
			Code:     "template-forbidden-type",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"template %q is declared on type %q which contains a %s field; templates cannot reference port/timer/default/map kinds (ETSI 15 / 6.2.15.1)",
				name, typeName, family),
			Node: td,
			Span: syntax.SpanOf(td),
		})
		return true
	})
	return diags
}
