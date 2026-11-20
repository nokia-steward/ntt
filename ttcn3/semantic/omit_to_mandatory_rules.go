// omit_to_mandatory_rules.go enforces ETSI ES 201 873-1
// clause 19.1 restriction c: if the left-hand side of an
// assignment refers to a non-optional value object, the
// right-hand side may not be the \`omit\` symbol nor
// reference an omitted field.
//
// We only flag the cleanest, fully literal shape:
//   - the LHS is a bare identifier referring to a sibling
//     local \`var T name\` declared earlier in the same
//     function body,
//   - T resolves to a module-local record / set declaration,
//   - the RHS is a positional CompositeLiteral of length
//     equal to the number of declared fields,
//   - one of the positional elements is the literal
//     \`omit\` whose matching declared field has no
//     \`optional\` annotation.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkOmitToMandatoryRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	records := collectRecordFieldLists(mod)
	if len(records) == 0 {
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
		scope := map[string]string{}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			if vd, ok := n.(*syntax.ValueDecl); ok && vd != nil &&
				vd.KindTok != nil && vd.KindTok.Kind() == syntax.VAR &&
				vd.TemplateRestriction == nil {
				if id, ok := vd.Type.(*syntax.Ident); ok && id != nil {
					if _, hit := records[id.String()]; hit {
						for _, dc := range vd.Decls {
							if dc != nil && dc.Name != nil {
								scope[dc.Name.String()] = id.String()
							}
							if dc != nil && dc.Value != nil {
								checkOmitInComposite(scope[dc.Name.String()], records, dc.Value, dc, &diags)
							}
						}
					}
				}
			}
			be, ok := n.(*syntax.BinaryExpr)
			if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhs, ok := be.X.(*syntax.Ident)
			if !ok || lhs == nil {
				return true
			}
			tname, ok := scope[lhs.String()]
			if !ok || tname == "" {
				return true
			}
			checkOmitInComposite(tname, records, be.Y, be, &diags)
			return true
		})
	}
	return diags
}

func checkOmitInComposite(
	tname string,
	records map[string][]recordField,
	rhs syntax.Expr,
	node syntax.Node,
	diags *[]Diagnostic,
) {
	fields := records[tname]
	if len(fields) == 0 {
		return
	}
	cl, ok := rhs.(*syntax.CompositeLiteral)
	if !ok || cl == nil {
		return
	}
	if len(cl.List) != len(fields) {
		return
	}
	for i, el := range cl.List {
		if !operandIsOmit(el) {
			continue
		}
		if fields[i].optional {
			continue
		}
		*diags = append(*diags, Diagnostic{
			Code:     "omit-to-mandatory-field",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`omit` cannot be assigned to mandatory field %q of record %q (ETSI 19.1 c)",
				fields[i].name, tname),
			Node: node,
			Span: syntax.SpanOf(node),
		})
		return
	}
}
