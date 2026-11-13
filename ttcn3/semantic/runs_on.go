package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// checkRunsOnReferences validates that every `runs on C` / `system C` /
// `mtc C` clause names a declared component type. The check walks the
// module rather than the global database because TTCN-3 mandates that the
// referenced component is visible from the function's owning module - via
// either local declaration or an `import from M { type C }` clause.
func (a *Analyzer) checkRunsOnReferences(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	report := func(spec string, expr syntax.Expr) {
		if expr == nil {
			return
		}
		name := syntax.Name(expr)
		if name == "" {
			return
		}
		if !a.componentResolves(tree, mod, name) {
			diags = append(diags, Diagnostic{
				Code:     "unknown-component",
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s clause references unknown component type %q", spec, name),
				Node:     expr,
				Span:     syntax.SpanOf(expr),
			})
		}
	}

	mod.Inspect(func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		if fn.RunsOn != nil {
			report("runs on", fn.RunsOn.Comp)
		}
		if fn.System != nil {
			report("system", fn.System.Comp)
		}
		if fn.Mtc != nil {
			report("mtc", fn.Mtc.Comp)
		}
		return true
	})
	return diags
}

// componentResolves returns true if name resolves to a type declaration
// reachable from mod. We look in three places, in this order:
//
//  1. The local module's top-level scope.
//  2. Any module the source imports from.
//  3. The full database, as a last-resort lenient fallback so that
//     incremental editing doesn't flag everything red while imports are
//     still missing.
func (a *Analyzer) componentResolves(tree *ttcn3.Tree, mod *syntax.Module, name string) bool {
	if defs := ttcn3.Definitions(name, mod, tree); len(defs) > 0 {
		return true
	}

	imported := importedModules(mod)

	// Same-file imported modules: a fixture frequently declares the
	// imported component type in a helper module within the same file
	// (e.g. `import from M_import { type GeneralComp }` where M_import
	// sits next to the test). The project DB may not index those, so
	// resolve them directly from the parsed tree before falling back to
	// the DB (Sem_08020301_GeneralFormatOfImport_009).
	if tree != nil {
		for _, modNode := range tree.Modules() {
			other, ok := modNode.Node.(*syntax.Module)
			if !ok || !imported[syntax.Name(other.Name)] {
				continue
			}
			if len(ttcn3.Definitions(name, other, tree)) > 0 {
				return true
			}
		}
	}

	if a.DB == nil {
		return false
	}

	for impName := range imported {
		files, ok := a.DB.Modules[impName]
		if !ok {
			continue
		}
		for f := range files {
			t := ttcn3.ParseFile(f)
			if t == nil || t.Root == nil {
				continue
			}
			for _, modNode := range t.Modules() {
				other, ok := modNode.Node.(*syntax.Module)
				if !ok || syntax.Name(other.Name) != impName {
					continue
				}
				if len(ttcn3.Definitions(name, other, t)) > 0 {
					return true
				}
			}
		}
	}
	// Lenient fallback: if any module declares the name, trust it. This
	// avoids cascade-of-red diagnostics while a project is half-imported.
	for _, files := range a.DB.Modules {
		for f := range files {
			t := ttcn3.ParseFile(f)
			if t == nil || t.Root == nil {
				continue
			}
			for _, modNode := range t.Modules() {
				other, ok := modNode.Node.(*syntax.Module)
				if !ok {
					continue
				}
				if len(ttcn3.Definitions(name, other, t)) > 0 {
					return true
				}
			}
		}
	}
	return false
}

// importedModules returns the set of module names that mod imports from,
// including the module itself so `runs on` references to types declared in
// the same module always resolve.
func importedModules(mod *syntax.Module) map[string]bool {
	out := map[string]bool{syntax.Name(mod.Name): true}
	mod.Inspect(func(n syntax.Node) bool {
		imp, ok := n.(*syntax.ImportDecl)
		if !ok {
			return true
		}
		if imp.Module == nil {
			return false
		}
		name := syntax.Name(imp.Module)
		if name != "" {
			out[name] = true
		}
		return false
	})
	return out
}
