// runs_on_missing_rules.go enforces ETSI ES 201 873-1 clause
// 16.1: a function that references a component-local
// variable / constant / timer / port must declare a
// compatible `runs on` clause. A plain `function` without a
// `runs on` cannot access the implicit `self` component.
//
// The check is intentionally narrow:
//   - We collect every named member declared inside every
//     component-body (var / const / template / timer / port).
//   - For each FuncDecl (function only, not testcase /
//     altstep / control) WITHOUT a `runs on` clause, we walk
//     the body and flag any Ident reference whose name is a
//     known component member but is NOT also a local
//     declaration, formal parameter, or module-level
//     identifier.
//
// This avoids false positives on functions that legitimately
// shadow a component-member name with a local var or
// parameter of the same name.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkRunsOnMissingRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	componentMembers := collectAllComponentMembers(mod)
	if len(componentMembers) == 0 {
		return nil
	}
	moduleSymbols := collectModuleSymbols(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil || fn.KindTok == nil {
			continue
		}
		// Testcases and altsteps may also reference
		// component members. Functions do too. All of them
		// require a `runs on` clause to use those members
		// (ETSI 16.1, 16.3 for testcases).
		kind := fn.KindTok.Kind()
		if kind != syntax.FUNCTION && kind != syntax.TESTCASE && kind != syntax.ALTSTEP {
			continue
		}
		if fn.RunsOn != nil {
			continue
		}
		params := collectFuncParamNames(fn)
		locals := collectFuncLocalNames(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			id, ok := n.(*syntax.Ident)
			if !ok || id == nil || id.Tok == nil {
				return true
			}
			name := id.String()
			if name == "" {
				return true
			}
			if params[name] || locals[name] || moduleSymbols[name] {
				return true
			}
			origins, isMember := componentMembers[name]
			if !isMember {
				return true
			}
			kindWord := "function"
			switch kind {
			case syntax.TESTCASE:
				kindWord = "testcase"
			case syntax.ALTSTEP:
				kindWord = "altstep"
			}
			diags = append(diags, Diagnostic{
				Code:     "runs-on-missing",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%s references component member %q (declared in component %s) but has no `runs on` clause (ETSI 16.1)",
					kindWord, name, joinNames(origins)),
				Node: id,
				Span: syntax.SpanOf(id),
			})
			return true
		})
	}
	return diags
}

// collectAllComponentMembers maps each member name to the
// list of component types that declare it.
func collectAllComponentMembers(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ct, ok := n.(*syntax.ComponentTypeDecl)
		if !ok || ct == nil || ct.Body == nil {
			return true
		}
		compName := ""
		if ct.Name != nil {
			compName = ct.Name.String()
		}
		for _, st := range ct.Body.Stmts {
			ds, ok := st.(*syntax.DeclStmt)
			if !ok || ds == nil {
				continue
			}
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if !ok || vd == nil {
				continue
			}
			for _, dec := range vd.Decls {
				if dec == nil || dec.Name == nil {
					continue
				}
				name := dec.Name.String()
				out[name] = append(out[name], compName)
			}
		}
		return true
	})
	return out
}

// collectModuleSymbols collects top-level definitions whose
// names could legitimately appear inside a function body
// without an associated runs-on clause (templates, consts,
// modulepars, functions, signatures, types, type aliases).
// We avoid scanning component-body members; those are
// handled separately so the runs-on check stays meaningful.
func collectModuleSymbols(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch v := d.Def.(type) {
		case *syntax.FuncDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
		case *syntax.SignatureDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
		case *syntax.TemplateDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
		case *syntax.ValueDecl:
			for _, dec := range v.Decls {
				if dec != nil && dec.Name != nil {
					out[dec.Name.String()] = true
				}
			}
		case *syntax.ModuleParameterGroup:
			for _, sub := range v.Decls {
				if sub == nil {
					continue
				}
				for _, dec := range sub.Decls {
					if dec != nil && dec.Name != nil {
						out[dec.Name.String()] = true
					}
				}
			}
		case *syntax.SubTypeDecl:
			if v.Field != nil && v.Field.Name != nil {
				out[v.Field.Name.String()] = true
			}
		case *syntax.StructTypeDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
		case *syntax.EnumTypeDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
			for _, e := range v.Enums {
				if id, ok := e.(*syntax.Ident); ok && id != nil {
					out[id.String()] = true
				}
				if ce, ok := e.(*syntax.CallExpr); ok && ce != nil {
					if id, ok := ce.Fun.(*syntax.Ident); ok && id != nil {
						out[id.String()] = true
					}
				}
			}
		case *syntax.PortTypeDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
		case *syntax.ComponentTypeDecl:
			if v.Name != nil {
				out[v.Name.String()] = true
			}
		}
	}
	return out
}

// collectFuncLocalNames returns the names of every local
// declarator anywhere inside the function body. Used to
// avoid flagging a reference that is actually bound to a
// local shadow.
func collectFuncLocalNames(body *syntax.BlockStmt) map[string]bool {
	out := map[string]bool{}
	if body == nil {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			for _, dec := range v.Decls {
				if dec != nil && dec.Name != nil {
					out[dec.Name.String()] = true
				}
			}
		case *syntax.FormalPar:
			if v != nil && v.Name != nil {
				out[v.Name.String()] = true
			}
		}
		return true
	})
	return out
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "?"
	}
	out := names[0]
	for _, n := range names[1:] {
		out += ", " + n
	}
	return out
}
