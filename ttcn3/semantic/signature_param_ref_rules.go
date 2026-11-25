// signature_param_ref_rules.go checks references to the parameters of a
// signature template (ETSI ES 201 873-1 clause 15.6.4): signature
// templates do not allow referencing their parameters directly, with dot
// notation; a modified template sets them with field assignments instead.
//
//	template S s_sig := { par1 := 1, par2 := 2 };
//	template integer t_par := s_sig.par1; // error
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSignatureParamRefRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*syntax.Ident)
		if !ok {
			return true
		}
		if sig := m.signatureOfTemplate(id); sig != "" {
			diags = append(diags, Diagnostic{
				Code:     "signature-template-parameter-reference",
				Severity: SeverityError,
				Message: fmt.Sprintf("%s: the parameters of the template %s of signature %s are not referenced directly (ETSI 15.6.4)",
					sourceText(sel), id, sig),
				Node: sel,
				Span: syntax.SpanOf(sel),
			})
		}
		return true
	})
	return diags
}

// signatureOfTemplate is the name of the signature the template
// definition id refers to is of, or "".
func (m typeModel) signatureOfTemplate(id *syntax.Ident) string {
	defs := m.tree.LookupWithDB(id, m.db)
	if len(defs) != 1 {
		return ""
	}
	td, ok := defs[0].Node.(*syntax.TemplateDecl)
	if !ok || td.Type == nil {
		return ""
	}
	types := defs[0].Tree.LookupWithDB(td.Type, m.db)
	if len(types) != 1 {
		return ""
	}
	if sig, ok := types[0].Node.(*syntax.SignatureDecl); ok && sig.Name != nil {
		return sig.Name.String()
	}
	return ""
}
