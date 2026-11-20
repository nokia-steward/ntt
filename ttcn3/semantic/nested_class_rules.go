// nested_class_rules.go enforces ETSI ES 201 873-1 clause 5.1.1.12
// restrictions on nested class declarations:
//
//   - Members (fields / methods / constructors) of a nested class
//     must not share a name with any member declared in any
//     transitively-enclosing class.
//
// We walk every ClassTypeDecl, build a stack of enclosing-class
// member names as we descend into nested ClassTypeDecls, and flag
// any name collision at the inner level.
//
// The check is purely syntactic - it inspects each class's `Defs`
// list and ignores inherited members (handled elsewhere). Members
// of an `extends` parent are not part of this rule's scope.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkNestedClassRules(mod *syntax.Module) []Diagnostic {
	index := indexModuleClasses(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ClassTypeDecl)
		if !ok || ct == nil {
			continue
		}
		diags = append(diags, walkNestedClasses(ct, nil, index)...)
		diags = append(diags, checkClassExtendsFinal(ct, index)...)
	}
	return diags
}

// checkClassExtendsFinal enforces ETSI 5.1.1.4: a class declared
// `@final` cannot appear in another class's `extends` list.
func checkClassExtendsFinal(ct *syntax.ClassTypeDecl, index map[string]*syntax.ClassTypeDecl) []Diagnostic {
	if ct == nil || len(ct.Extends) == 0 {
		return nil
	}
	var diags []Diagnostic
	for _, e := range ct.Extends {
		id, ok := e.(*syntax.Ident)
		if !ok || id == nil {
			continue
		}
		parent := index[id.String()]
		if parent == nil || parent.Modif == nil {
			continue
		}
		if parent.Modif.String() != "@final" {
			continue
		}
		name := ""
		if ct.Name != nil {
			name = ct.Name.String()
		}
		diags = append(diags, Diagnostic{
			Code:     "class-extends-final",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"class %q cannot extend `@final` class %q (ETSI 5.1.1.4)",
				name, id.String()),
			Node: ct,
			Span: syntax.SpanOf(ct),
		})
	}
	return diags
}

// indexModuleClasses returns a map of every class name declared at
// module scope (top-level only, since nested classes are not
// referencable by bare identifier elsewhere) to its ClassTypeDecl.
// Used to follow `extends` chains when computing the enclosing-class
// member set for nested-class shadowing checks.
func indexModuleClasses(mod *syntax.Module) map[string]*syntax.ClassTypeDecl {
	out := map[string]*syntax.ClassTypeDecl{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ClassTypeDecl)
		if !ok || ct == nil || ct.Name == nil {
			continue
		}
		out[ct.Name.String()] = ct
	}
	return out
}

// walkNestedClasses descends through `class` definitions. For each
// inner class it compares the member names declared at its level
// against the union of every outer class's member names (including
// names inherited via the outer class's `extends` chain).
func walkNestedClasses(ct *syntax.ClassTypeDecl, enclosing []classMemberScope, index map[string]*syntax.ClassTypeDecl) []Diagnostic {
	if ct == nil || ct.Name == nil {
		return nil
	}
	myMembers := classMemberNames(ct)
	var diags []Diagnostic
	for name, node := range myMembers {
		for _, outer := range enclosing {
			if _, clash := outer.members[name]; clash {
				diags = append(diags, Diagnostic{
					Code:     "nested-class-member-shadow",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"nested class %q member %q shadows a member of enclosing class %q (ETSI 5.1.1.12)",
						ct.Name.String(), name, outer.name),
					Node: node,
					Span: syntax.SpanOf(node),
				})
			}
		}
	}
	combined := classMembersWithInherited(ct, index, map[string]bool{})
	scope := append(enclosing, classMemberScope{name: ct.Name.String(), members: combined})
	for _, def := range ct.Defs {
		if def == nil {
			continue
		}
		inner, ok := def.Def.(*syntax.ClassTypeDecl)
		if !ok || inner == nil {
			continue
		}
		diags = append(diags, walkNestedClasses(inner, scope, index)...)
	}
	return diags
}

// classMembersWithInherited returns the union of ct's direct
// members and the members of every class reachable via its
// transitive `extends` chain. `visited` guards against cycles.
func classMembersWithInherited(ct *syntax.ClassTypeDecl, index map[string]*syntax.ClassTypeDecl, visited map[string]bool) map[string]syntax.Node {
	out := map[string]syntax.Node{}
	if ct == nil || ct.Name == nil {
		return out
	}
	if visited[ct.Name.String()] {
		return out
	}
	visited[ct.Name.String()] = true
	for k, v := range classMemberNames(ct) {
		out[k] = v
	}
	for _, e := range ct.Extends {
		id, ok := e.(*syntax.Ident)
		if !ok || id == nil {
			continue
		}
		parent, ok := index[id.String()]
		if !ok || parent == nil {
			continue
		}
		for k, v := range classMembersWithInherited(parent, index, visited) {
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
	}
	return out
}

type classMemberScope struct {
	name    string
	members map[string]syntax.Node
}

func classMemberNames(ct *syntax.ClassTypeDecl) map[string]syntax.Node {
	out := map[string]syntax.Node{}
	if ct == nil {
		return out
	}
	for _, def := range ct.Defs {
		if def == nil || def.Def == nil {
			continue
		}
		switch v := def.Def.(type) {
		case *syntax.ValueDecl:
			if v == nil {
				continue
			}
			for _, d := range v.Decls {
				if d == nil || d.Name == nil {
					continue
				}
				out[d.Name.String()] = d
			}
		case *syntax.FuncDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out[v.Name.String()] = v
		case *syntax.ConstructorDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out[v.Name.String()] = v
		case *syntax.TemplateDecl:
			if v == nil || v.Name == nil {
				continue
			}
			out[v.Name.String()] = v
		}
	}
	return out
}
