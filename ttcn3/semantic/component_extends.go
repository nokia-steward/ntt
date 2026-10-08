// component_extends.go implements ETSI ES 201 873-1 clause 6.2.10
// component-type extension rules:
//
//   - Restriction b: extending more than one parent type must not
//     introduce name clashes between the parents' member sets.
//     Emits component-extends-name-clash.
//   - Restriction c: there shall be no cyclic chain of extension
//     definitions. Emits component-extends-cycle.
//   - Adjunct: a member declared in the child whose name matches a
//     member inherited from a parent is also a clash (the spec
//     wording in 6.2.10 lists this under the same restriction). We
//     re-use component-extends-name-clash for this case.
//
// All checks are syntactic: we collect the immediate parent list
// for every named component type, the local member name set for
// each component body, then run a fixpoint to detect cycles. The
// name-clash walker compares the merged parent-member sets to the
// child's own members.
package semantic

import (
	"fmt"
	"sort"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// componentMeta records the parent identifiers and own member
// names of a component type declaration. ownMembers omits inherited
// members.
type componentMeta struct {
	parents    []string
	ownMembers map[string]bool
	decl       *syntax.ComponentTypeDecl
}

// checkComponentExtendsRules is wired into Analyze; emits the
// diagnostics described at the top of this file.
func (a *Analyzer) checkComponentExtendsRules(mod *syntax.Module) []Diagnostic {
	comps := collectComponentMeta(mod)
	if len(comps) == 0 {
		return nil
	}
	var diags []Diagnostic
	diags = append(diags, checkComponentCycles(comps)...)
	diags = append(diags, checkComponentNameClashes(comps)...)
	return diags
}

// collectComponentMeta walks top-level type decls and records
// each ComponentTypeDecl's parent list and local members.
func collectComponentMeta(mod *syntax.Module) map[string]componentMeta {
	out := map[string]componentMeta{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		cd, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || cd.Name == nil {
			continue
		}
		out[cd.Name.String()] = componentMeta{
			parents:    componentParentNames(cd),
			ownMembers: componentOwnMembers(cd),
			decl:       cd,
		}
	}
	return out
}

// componentParentNames extracts the Ident names from an
// extension clause. Non-Ident shapes (parameterised refs etc.)
// are skipped.
func componentParentNames(cd *syntax.ComponentTypeDecl) []string {
	var out []string
	for _, e := range cd.Extends {
		if name := identName(e); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// componentOwnMembers returns the set of member names (ports +
// vars + timers + constants) declared in the component body, not
// counting inherited members.
func componentOwnMembers(cd *syntax.ComponentTypeDecl) map[string]bool {
	out := map[string]bool{}
	if cd == nil || cd.Body == nil {
		return out
	}
	for _, st := range cd.Body.Stmts {
		ds, ok := st.(*syntax.DeclStmt)
		if !ok || ds == nil {
			continue
		}
		for _, name := range declStmtNames(ds.Decl) {
			out[name] = true
		}
	}
	return out
}

// declStmtNames returns every identifier introduced by a single
// declaration statement (which for component bodies is almost
// always a ValueDecl wrapping `port`, `var`, `timer`, `const`).
func declStmtNames(d syntax.Decl) []string {
	if x, ok := d.(*syntax.ValueDecl); ok && x != nil {
		var out []string
		for _, dec := range x.Decls {
			if dec != nil && dec.Name != nil {
				out = append(out, dec.Name.String())
			}
		}
		return out
	}
	return nil
}

// checkComponentCycles emits component-extends-cycle for every
// component whose extension chain forms a cycle. We do a DFS from
// each comp; the first time we revisit a node already on the
// current path we record a diagnostic at the cycle entry point.
func checkComponentCycles(comps map[string]componentMeta) []Diagnostic {
	var diags []Diagnostic
	reported := map[string]bool{}
	for name := range comps {
		if reported[name] {
			continue
		}
		path := map[string]bool{}
		if cycle := dfsComponentCycle(name, comps, path, nil); cycle != nil {
			// Report the diagnostic only against the cycle
			// entry to avoid n duplicates per ring.
			head := cycle[0]
			if reported[head] {
				continue
			}
			reported[head] = true
			diags = append(diags, Diagnostic{
				Code:     "component-extends-cycle",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"cyclic component extension: %s",
					formatComponentCycle(cycle)),
				Node: comps[head].decl,
				Span: syntax.SpanOf(comps[head].decl),
			})
		}
	}
	return diags
}

// dfsComponentCycle recursively walks the extends-DAG, returning
// the cycle path (start..repeat) when one is found.
func dfsComponentCycle(
	name string,
	comps map[string]componentMeta,
	onPath map[string]bool,
	stack []string,
) []string {
	if onPath[name] {
		// Slice the stack from the first occurrence of `name`
		// to here so the returned path is exactly the cycle.
		for i, n := range stack {
			if n == name {
				return append(append([]string{}, stack[i:]...), name)
			}
		}
		return append(stack, name)
	}
	meta, ok := comps[name]
	if !ok {
		return nil
	}
	onPath[name] = true
	stack = append(stack, name)
	for _, p := range meta.parents {
		if cycle := dfsComponentCycle(p, comps, onPath, stack); cycle != nil {
			return cycle
		}
	}
	delete(onPath, name)
	return nil
}

// formatComponentCycle renders a cycle as `A -> B -> C -> A`.
func formatComponentCycle(cycle []string) string {
	if len(cycle) == 0 {
		return ""
	}
	out := cycle[0]
	for _, n := range cycle[1:] {
		out += " -> " + n
	}
	return out
}

// checkComponentNameClashes emits component-extends-name-clash
// when two parents of the same component contribute a member with
// the same name, or when a child member redeclares an inherited
// one. Cycles are skipped to avoid infinite recursion when the
// parent set isn't a DAG.
func checkComponentNameClashes(comps map[string]componentMeta) []Diagnostic {
	var diags []Diagnostic
	cyclic := componentsInCycles(comps)
	for name, meta := range comps {
		if len(meta.parents) == 0 || cyclic[name] {
			continue
		}
		// inheritedFrom[member] -> list of parents contributing it
		inheritedFrom := map[string][]string{}
		for _, parent := range meta.parents {
			pSet := flatMembers(parent, comps, cyclic, map[string]bool{})
			for member := range pSet {
				inheritedFrom[member] = append(inheritedFrom[member], parent)
			}
		}
		// Parent-vs-parent clash:
		for member, sources := range inheritedFrom {
			if len(sources) > 1 {
				sort.Strings(sources)
				diags = append(diags, Diagnostic{
					Code:     "component-extends-name-clash",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"component %q inherits member %q from multiple parents: %v",
						name, member, sources),
					Node: meta.decl,
					Span: syntax.SpanOf(meta.decl),
				})
			}
		}
		// Child-vs-inherited clash:
		for ownMember := range meta.ownMembers {
			if sources, ok := inheritedFrom[ownMember]; ok && len(sources) > 0 {
				sort.Strings(sources)
				diags = append(diags, Diagnostic{
					Code:     "component-extends-name-clash",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"component %q redeclares member %q already inherited from %v",
						name, ownMember, sources),
					Node: meta.decl,
					Span: syntax.SpanOf(meta.decl),
				})
			}
		}
	}
	return diags
}

// flatMembers returns the transitive member set of `name` walking
// the extends-DAG. Cyclic components are skipped (they're handled
// by checkComponentCycles).
func flatMembers(
	name string,
	comps map[string]componentMeta,
	cyclic map[string]bool,
	visited map[string]bool,
) map[string]bool {
	if visited[name] || cyclic[name] {
		return nil
	}
	visited[name] = true
	meta, ok := comps[name]
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for m := range meta.ownMembers {
		out[m] = true
	}
	for _, p := range meta.parents {
		for m := range flatMembers(p, comps, cyclic, visited) {
			out[m] = true
		}
	}
	return out
}

// componentsInCycles returns the set of components that are part
// of an extension cycle, computed from the same DFS used by
// checkComponentCycles.
func componentsInCycles(comps map[string]componentMeta) map[string]bool {
	out := map[string]bool{}
	for name := range comps {
		if out[name] {
			continue
		}
		path := map[string]bool{}
		if cycle := dfsComponentCycle(name, comps, path, nil); cycle != nil {
			for _, n := range cycle {
				out[n] = true
			}
		}
	}
	return out
}
