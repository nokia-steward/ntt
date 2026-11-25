// type_def_ref_rules.go checks what type and array definitions refer to
// (ETSI ES 201 873-1 clauses 6.2.7, 8.2.1):
//
//   - a module parameter is not used within a type definition of a data
//     type (8.2.1);
//   - the dimension of an array variable or constant is not a formal
//     parameter of the function, altstep, testcase or template it is
//     declared in: the number of array elements is fixed (6.2.7).
//
// A union alternative of the union type itself is valid as long as
// another alternative is not (6.2), so 6.2.5.1 is not checked here.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTypeDefRefRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	db := a.db()
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	resolve := func(id *syntax.Ident) syntax.Node {
		defs := tree.LookupWithDB(id, db)
		if len(defs) != 1 {
			return nil
		}
		return defs[0].Node
	}
	behaviourPars := map[*syntax.FormalPar]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		var pars *syntax.FormalPars
		switch x := n.(type) {
		case *syntax.FuncDecl:
			pars = x.Params
		case *syntax.TemplateDecl:
			pars = x.Params
		}
		if pars != nil {
			for _, p := range pars.List {
				behaviourPars[p] = true
			}
		}
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.SubTypeDecl, *syntax.StructTypeDecl, *syntax.EnumTypeDecl, *syntax.MapTypeDecl:
			syntax.Inspect(x, func(n syntax.Node) bool {
				id, ok := n.(*syntax.Ident)
				if !ok || id.IsName {
					return true
				}
				if vd, ok := resolve(id).(*syntax.ValueDecl); ok && vd.KindTok != nil && vd.KindTok.Kind() == syntax.MODULEPAR {
					add(id, "modulepar-in-type-definition", fmt.Sprintf("module parameter %s is used within a type definition (ETSI 8.2.1)", id))
				}
				return true
			})
			return false
		case *syntax.Declarator:
			for _, dim := range x.ArrayDef {
				for _, e := range dim.List {
					syntax.Inspect(e, func(n syntax.Node) bool {
						id, ok := n.(*syntax.Ident)
						if !ok {
							return true
						}
						if p, ok := resolve(id).(*syntax.FormalPar); ok && behaviourPars[p] {
							add(id, "array-dim-non-const", fmt.Sprintf("array dimension %s is a formal parameter; the number of array elements is fixed (ETSI 6.2.7)", id))
						}
						return true
					})
				}
			}
		}
		return true
	})
	return diags
}
