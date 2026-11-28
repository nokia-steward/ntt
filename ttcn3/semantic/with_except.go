package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkWithExceptRefs verifies that every reference inside an AllRef
// exclusion (`with { encode (const all except {x}) "Rule" }`) names a
// definition of the scope the with statement is associated with
// (ETSI ES 201 873-1, 27.2). Naming a non-existent item - or one that
// lives outside the group the clause is attached to - is an error.
func (a *Analyzer) checkWithExceptRefs(mod *syntax.Module) []Diagnostic {
	var out []Diagnostic

	check := func(ws *syntax.WithSpec, defs []*syntax.ModuleDef) {
		if ws == nil {
			return
		}
		names := map[string]bool{}
		var collect func(ds []*syntax.ModuleDef)
		collect = func(ds []*syntax.ModuleDef) {
			for _, d := range ds {
				if d == nil || d.Def == nil {
					continue
				}
				if g, ok := d.Def.(*syntax.GroupDecl); ok {
					if g.Name != nil {
						names[g.Name.String()] = true
					}
					collect(g.Defs)
					continue
				}
				for _, name := range moduleDefNames(d.Def) {
					names[name] = true
				}
			}
		}
		collect(defs)
		for _, stmt := range ws.List {
			if stmt == nil {
				continue
			}
			for _, q := range stmt.List {
				dk, ok := q.(*syntax.DefKindExpr)
				if !ok || dk == nil {
					continue
				}
				for _, e := range dk.List {
					ex, ok := e.(*syntax.ExceptExpr)
					if !ok || ex == nil {
						continue
					}
					for _, r := range ex.List {
						name := flatRefName(r)
						if name == "" || names[name] {
							continue
						}
						out = append(out, Diagnostic{
							Code:     "attr.except-unknown",
							Severity: SeverityError,
							Message: fmt.Sprintf(
								"`except` references %q, which is not defined in the scope of this `with` statement",
								name),
							Span: syntax.SpanOf(r),
						})
					}
				}
			}
		}
	}

	check(mod.With, mod.Defs)
	mod.Inspect(func(n syntax.Node) bool {
		if g, ok := n.(*syntax.GroupDecl); ok {
			check(g.With, g.Defs)
		}
		return true
	})
	return out
}

// moduleDefNames lists the names a module definition introduces into
// its scope. Multi-declarator value declarations contribute every
// declarator name.
func moduleDefNames(def syntax.Node) []string {
	switch x := def.(type) {
	case *syntax.ValueDecl:
		var names []string
		for _, d := range x.Decls {
			if d != nil && d.Name != nil {
				names = append(names, syntax.Name(d.Name))
			}
		}
		return names
	case *syntax.TemplateDecl:
		if x.Name != nil {
			return []string{syntax.Name(x.Name)}
		}
	case *syntax.FuncDecl:
		if x.Name != nil {
			return []string{x.Name.String()}
		}
	default:
		if name := syntax.Name(def); name != "" {
			return []string{name}
		}
	}
	return nil
}

// flatRefName flattens an except-list reference to its identifier
// form; qualified references keep only the last segment because the
// exclusion is interpreted in the local scope.
func flatRefName(e syntax.Expr) string {
	switch r := e.(type) {
	case *syntax.Ident:
		return r.String()
	case *syntax.SelectorExpr:
		return flatRefName(r.Sel)
	}
	return ""
}
