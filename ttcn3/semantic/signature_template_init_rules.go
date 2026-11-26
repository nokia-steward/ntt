// signature_template_init_rules.go checks what a signature template
// initialises (ETSI ES 201 873-1 clauses 15.2 and 15.3 e): its parameters
// only, not the return value, which a reply or getreply gives; and either
// all of them, or all in and inout parameters with no out parameter, or
// all out and inout parameters with no in parameter, a parameter set to
// the not used symbol counting as not initialised. An @abstract template
// may initialise less.
//
//	signature S(in integer p1, out integer p2, inout integer p3);
//	template S s_t := { p1 := 1, p2 := 2, p3 := - }; // error
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSignatureTemplateInitRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td.ModifiesTok != nil || td.Type == nil {
			return true
		}
		cl, ok := td.Value.(*syntax.CompositeLiteral)
		if !ok {
			return true
		}
		defs := tree.LookupWithDB(td.Type, m.db)
		if len(defs) != 1 {
			return true
		}
		sig, ok := defs[0].Node.(*syntax.SignatureDecl)
		if !ok || sig.Params == nil {
			return true
		}
		params := sig.Params.List
		name := syntax.Name(td.Name)
		set := map[string]bool{}
		for i, el := range cl.List {
			if be, ok := el.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
				if !isNotUsedSymbol(be.Y) {
					set[syntax.Name(be.X)] = true
				}
				continue
			}
			if i >= len(params) {
				add(el, "signature-template-return-value", fmt.Sprintf("template %s gives %s %d values for its %d parameters: the return value is no part of a signature template (ETSI 15.2)",
					name, syntax.Name(sig.Name), len(cl.List), len(params)))
				return true
			}
			if params[i] != nil && !isNotUsedSymbol(el) {
				set[syntax.Name(params[i].Name)] = true
			}
		}
		all, inSide, outSide := true, true, true
		for _, p := range params {
			if p == nil {
				continue
			}
			dir := syntax.IN
			if p.Direction != nil {
				dir = p.Direction.Kind()
			}
			init := set[syntax.Name(p.Name)]
			all = all && init
			switch dir {
			case syntax.IN:
				inSide = inSide && init
				outSide = outSide && !init
			case syntax.OUT:
				inSide = inSide && !init
				outSide = outSide && init
			case syntax.INOUT:
				inSide = inSide && init
				outSide = outSide && init
			}
		}
		abstract := td.Modif != nil && td.Modif.String() == "@abstract"
		if !all && !inSide && !outSide && !abstract {
			add(td, "signature-template-partial", fmt.Sprintf("template %s initialises neither all parameters of %s, nor all its in and inout parameters alone, nor all its out and inout parameters alone (ETSI 15.3 e)",
				name, syntax.Name(sig.Name)))
		}
		return true
	})
	return diags
}
