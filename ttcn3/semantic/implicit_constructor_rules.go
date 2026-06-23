// implicit_constructor_rules.go enforces ETSI ES 201 873-1
// clause 5.1.1.5 restrictions on implicit class constructors:
//
//   - A class without an explicit `create(...)` declaration has an
//     implicit constructor whose formal parameter list is the set
//     of the class's un-initialised `var` / `const` / `template`
//     fields, in declaration order. Calls to such a class's
//     `<Class>.create(arg, ...)` must pass exactly that many
//     arguments.
//
// We only fire when:
//   - the class is module-local, has no explicit ConstructorDecl,
//     and isn't `@abstract`;
//   - the call's callee resolves to `<Class>.create` literally
//     (not a parametric class reference or a field selector).
//
// Anything else stays silent.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkImplicitConstructorRules(mod *syntax.Module) []Diagnostic {
	classes := collectClassConstructorMeta(mod)
	if len(classes) == 0 {
		return nil
	}
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		sel, ok := ce.Fun.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		base, ok := sel.X.(*syntax.Ident)
		if !ok || base == nil {
			return true
		}
		op, ok := sel.Sel.(*syntax.Ident)
		if !ok || op == nil || op.String() != "create" {
			return true
		}
		meta, ok := classes[base.String()]
		if !ok || !meta.implicit {
			return true
		}
		var argCount int
		if ce.Args != nil {
			argCount = len(ce.Args.List)
		}
		if argCount == meta.uninitFields {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "implicit-constructor-arity",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"%s.create takes %d argument(s) (the class's un-initialised fields), got %d (ETSI 5.1.1.5)",
				base.String(), meta.uninitFields, argCount),
			Node: ce,
			Span: syntax.SpanOf(ce),
		})
		return true
	})
	return diags
}

type classConstructorMeta struct {
	implicit     bool
	uninitFields int
}

func collectClassConstructorMeta(mod *syntax.Module) map[string]classConstructorMeta {
	index := indexModuleClasses(mod)
	out := map[string]classConstructorMeta{}
	for name, ct := range index {
		if ct == nil {
			continue
		}
		if ct.Modif != nil && ct.Modif.String() == "@abstract" {
			continue
		}
		hasExplicit, _ := classHasExplicitCtor(ct)
		if hasExplicit {
			out[name] = classConstructorMeta{implicit: false}
			continue
		}
		out[name] = classConstructorMeta{
			implicit:     true,
			uninitFields: classUninitFieldCount(ct, index, map[string]bool{}),
		}
	}
	return out
}

func classHasExplicitCtor(ct *syntax.ClassTypeDecl) (bool, *syntax.ConstructorDecl) {
	if ct == nil {
		return false, nil
	}
	for _, def := range ct.Defs {
		if def == nil || def.Def == nil {
			continue
		}
		if cd, ok := def.Def.(*syntax.ConstructorDecl); ok && cd != nil {
			return true, cd
		}
	}
	return false, nil
}

// classUninitFieldCount returns the number of un-initialised
// var / const / template members of ct, including those reached
// through its `extends` chain. The walk halts on cycles via
// `visited`.
func classUninitFieldCount(ct *syntax.ClassTypeDecl, index map[string]*syntax.ClassTypeDecl, visited map[string]bool) int {
	if ct == nil || ct.Name == nil || visited[ct.Name.String()] {
		return 0
	}
	visited[ct.Name.String()] = true
	count := 0
	for _, def := range ct.Defs {
		if def == nil || def.Def == nil {
			continue
		}
		switch v := def.Def.(type) {
		case *syntax.ValueDecl:
			if v == nil || v.KindTok == nil {
				continue
			}
			switch v.KindTok.Kind() {
			case syntax.VAR, syntax.CONST:
			default:
				continue
			}
			for _, dd := range v.Decls {
				if dd == nil {
					continue
				}
				if dd.Value == nil {
					count++
				}
			}
		case *syntax.TemplateDecl:
			if v == nil {
				continue
			}
			if v.Value == nil {
				count++
			}
		}
	}
	for _, e := range ct.Extends {
		id, ok := e.(*syntax.Ident)
		if !ok || id == nil {
			continue
		}
		parent := index[id.String()]
		if parent == nil {
			continue
		}
		count += classUninitFieldCount(parent, index, visited)
	}
	return count
}
