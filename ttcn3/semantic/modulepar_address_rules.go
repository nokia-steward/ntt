// modulepar_address_rules.go enforces ETSI ES 201 873-1
// clause 8.2.1: "A module parameter shall only be of type
// `address` if the address type is explicitly defined within
// the associated module."
//
// We flag every modulepar whose type identifier is the bare
// `address` keyword when no top-level `type ... address`
// declaration exists in the same module.
package semantic

import (
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkModuleparAddressRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	if moduleDefinesAddress(mod) {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		switch v := d.Def.(type) {
		case *syntax.ValueDecl:
			collectModuleparAddressDiags(v, &diags)
		case *syntax.ModuleParameterGroup:
			if v == nil {
				continue
			}
			for _, sub := range v.Decls {
				collectModuleparAddressDiags(sub, &diags)
			}
		}
	}
	return diags
}

func collectModuleparAddressDiags(vd *syntax.ValueDecl, diags *[]Diagnostic) {
	if vd == nil || vd.KindTok == nil {
		return
	}
	if vd.KindTok.Kind() != syntax.MODULEPAR {
		return
	}
	if !typeIsAddress(vd.Type) {
		return
	}
	for _, dec := range vd.Decls {
		if dec == nil || dec.Name == nil {
			continue
		}
		*diags = append(*diags, Diagnostic{
			Code:     "modulepar-address-undefined",
			Severity: SeverityError,
			Message: "modulepar " + quoted(dec.Name.String()) +
				": type `address` is referenced but no `type ... address` declaration exists in this module (ETSI 8.2.1)",
			Node: vd,
			Span: syntax.SpanOf(vd),
		})
	}
}

func typeIsAddress(t syntax.Expr) bool {
	id, ok := t.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return false
	}
	if id.Tok.Kind() == syntax.ADDRESS {
		return true
	}
	return id.String() == "address"
}

func moduleDefinesAddress(mod *syntax.Module) bool {
	defines := false
	syntax.Inspect(mod, func(n syntax.Node) bool {
		if defines {
			return false
		}
		st, ok := n.(*syntax.SubTypeDecl)
		if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
			return true
		}
		if st.Field.Name.String() == "address" {
			defines = true
			return false
		}
		return true
	})
	return defines
}
