// template_init_complete_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 15.3 restriction d): after
// completing initialisation, every non-@abstract template
// must be fully initialised. We only flag the cleanest
// missing-named-field shape:
//
//   - the template type resolves directly to a local record /
//     set declaration with a known field list,
//   - the template body is a CompositeLiteral built from
//     named field assignments only (no positional shorthand,
//     no `...`, no NotUsed symbol),
//   - the template carries no `modifies` clause and no
//     attribute that could plausibly mark it abstract,
//   - the template carries no formal parameters whose default
//     value supplies the missing field.
//
// Any required (non-optional) field that is absent from the
// named-assignment list is reported. Optional fields may be
// omitted.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTemplateInitCompleteRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	records := collectRecordFieldLists(mod)
	if len(records) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		if td.ModifiesTok != nil || td.Modif != nil {
			return true
		}
		typeID, ok := td.Type.(*syntax.Ident)
		if !ok || typeID == nil {
			return true
		}
		fields, ok := records[typeID.String()]
		if !ok || len(fields) == 0 {
			return true
		}
		cl, ok := td.Value.(*syntax.CompositeLiteral)
		if !ok || cl == nil || len(cl.List) == 0 {
			return true
		}
		seen := map[string]bool{}
		anyNamed := false
		for _, item := range cl.List {
			be, ok := item.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			id, ok := be.X.(*syntax.Ident)
			if !ok || id == nil {
				return true
			}
			seen[id.String()] = true
			anyNamed = true
		}
		if !anyNamed {
			return true
		}
		var missing []string
		for _, f := range fields {
			if f.optional {
				continue
			}
			if !seen[f.name] {
				missing = append(missing, f.name)
			}
		}
		if len(missing) == 0 {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "template-incomplete-initialisation",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"template %q omits required field(s) %v of type %q; non-@abstract templates must be fully initialised (ETSI 15.3 d)",
				td.Name.String(), missing, typeID.String()),
			Node: td,
			Span: syntax.SpanOf(td),
		})
		return true
	})
	return diags
}

type recordField struct {
	name     string
	optional bool
}

func collectRecordFieldLists(mod *syntax.Module) map[string][]recordField {
	out := map[string][]recordField{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		std, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || std == nil || std.Name == nil || std.KindTok == nil {
			continue
		}
		switch std.KindTok.Kind() {
		case syntax.RECORD, syntax.SET:
		default:
			continue
		}
		var fields []recordField
		for _, f := range std.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			fields = append(fields, recordField{
				name:     f.Name.String(),
				optional: f.Optional != nil,
			})
		}
		out[std.Name.String()] = fields
	}
	return out
}
