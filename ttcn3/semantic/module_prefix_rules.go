// module_prefix_rules.go enforces ETSI ES 201 873-1 clause 8.2.3.1: the
// identifier of the current module may prefix a definition referenced in
// it only when the definition has global visibility for the module, that
// is, when it is one of the module's own definitions. A local variable,
// a component member or an imported definition cannot be qualified with
// the name of the module that uses it.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkModulePrefixRules(mod *syntax.Module) []Diagnostic {
	if mod == nil || mod.Name == nil {
		return nil
	}
	self := mod.Name.String()
	global := map[string]bool{}
	// unnamed records a definition whose name is not known here, so the
	// set of global names may be incomplete.
	unnamed := false
	var collect func(defs []*syntax.ModuleDef)
	collect = func(defs []*syntax.ModuleDef) {
		for _, d := range defs {
			if d == nil || d.Def == nil {
				continue
			}
			switch x := d.Def.(type) {
			case *syntax.GroupDecl:
				collect(x.Defs)
			case *syntax.ImportDecl, *syntax.FriendDecl:
			case *syntax.ValueDecl:
				for _, dec := range x.Decls {
					if dec != nil && dec.Name != nil {
						global[dec.Name.String()] = true
					}
				}
			case *syntax.ModuleParameterGroup:
				for _, vd := range x.Decls {
					for _, dec := range vd.Decls {
						if dec != nil && dec.Name != nil {
							global[dec.Name.String()] = true
						}
					}
				}
			case *syntax.ClassTypeDecl:
				global[syntax.Name(x.Name)] = true
			case *syntax.ControlPart:
				global["control"] = true
			default:
				if name := syntax.Name(d.Def); name != "" {
					global[name] = true
				} else {
					unnamed = true
				}
			}
		}
	}
	collect(mod.Defs)

	// A name declared anywhere as the module's own name would make the
	// prefix ambiguous; leave such modules alone.
	shadowed := false
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.Declarator:
			shadowed = shadowed || syntax.Name(x.Name) == self
		case *syntax.FormalPar:
			shadowed = shadowed || syntax.Name(x.Name) == self
		}
		return !shadowed
	})
	if unnamed || shadowed || global[self] {
		return nil
	}

	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base.Tok == nil || base.Tok2 != nil || base.String() != self {
			return true
		}
		name := syntax.Name(sel.Sel)
		if name == "" || global[name] {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "module-prefix-not-global",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s.%s: the module's own name may only prefix its module-level definitions, and %q is not one (ETSI 8.2.3.1)",
				self, name, name),
			Node: sel,
			Span: syntax.SpanOf(sel),
		})
		return true
	})
	return diags
}
