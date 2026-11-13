// identifier_uniqueness.go enforces ETSI ES 201 873-1 clause
// 5.2.2 "Uniqueness of identifiers":
//
//	Within the same scope, identifier names shall be unique.
//	A locally declared identifier shall not shadow an outer
//	identifier visible in the same scope (module-level defs,
//	the runs-on component's members, the function's formal
//	parameters, or the enclosing module's own name).
//
// We catch the static shapes:
//
//   - duplicate-identifier-in-scope: two var/const/template
//     declarations in the same function/testcase body share a
//     name (also covers param-vs-body collision).
//   - identifier-shadows-module-def: a body-local var/const
//     shadows a module-level const/modulepar/function/template/
//     testcase/signature/type or the module's own name.
//   - identifier-shadows-component-member: a body-local
//     var/const shadows a member of the function's runs-on
//     component.
//
// Class members and inheritance are handled separately in
// component_ops.go; here we focus on function/testcase scopes.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkIdentifierUniqueness(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic

	moduleName := ""
	if mod.Name != nil {
		moduleName = mod.Name.String()
	}
	moduleDefs := collectModuleLevelNames(mod)
	componentMembers := collectAllComponentMemberNames(mod)
	parents := collectComponentParents(mod)
	componentMembers = flattenComponentMembers(componentMembers, parents)

	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		comp := ""
		if fn.RunsOn != nil && fn.RunsOn.Comp != nil {
			comp = syntax.Name(fn.RunsOn.Comp)
		}
		paramNames := collectFormalParamNames(fn.Params)
		diags = append(diags, checkBodyIdentifierUniqueness(
			fn, paramNames, moduleDefs, componentMembers[comp], moduleName)...)
	}
	return diags
}

// checkBodyIdentifierUniqueness walks a function/testcase body
// with a manual scope stack so per-block scopes are respected:
// two `for (var integer i := 0; ...)` loops in sequence both
// declare i in their own scope and don't conflict. The walker
// pushes a new scope on every BlockStmt, ForStmt, WhileStmt,
// DoStmt, IfStmt branch, AltStmt and CommClause.
func checkBodyIdentifierUniqueness(
	fn *syntax.FuncDecl,
	paramNames map[string]bool,
	moduleDefs map[string]bool,
	componentMembers map[string]bool,
	moduleName string,
) []Diagnostic {
	var diags []Diagnostic
	scopes := []map[string]bool{{}}
	push := func() { scopes = append(scopes, map[string]bool{}) }
	pop := func() { scopes = scopes[:len(scopes)-1] }
	inAnyScope := func(name string) bool {
		for _, s := range scopes {
			if s[name] {
				return true
			}
		}
		return false
	}
	declare := func(dec *syntax.Declarator) {
		if dec == nil || dec.Name == nil {
			return
		}
		name := dec.Name.String()
		top := scopes[len(scopes)-1]
		if top[name] {
			diags = append(diags, Diagnostic{
				Code:     "duplicate-identifier-in-scope",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q is already declared earlier in this scope (ETSI 5.2.2)",
					name),
				Node: dec,
				Span: syntax.SpanOf(dec),
			})
			return
		}
		if paramNames[name] {
			diags = append(diags, Diagnostic{
				Code:     "duplicate-identifier-in-scope",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q shadows a formal parameter of the same name (ETSI 5.2.2)",
					name),
				Node: dec,
				Span: syntax.SpanOf(dec),
			})
			top[name] = true
			return
		}
		if componentMembers[name] && !inAnyScope(name) {
			diags = append(diags, Diagnostic{
				Code:     "identifier-shadows-component-member",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"%q shadows a member of the runs-on component (ETSI 5.2.2)",
					name),
				Node: dec,
				Span: syntax.SpanOf(dec),
			})
			top[name] = true
			return
		}
		if !inAnyScope(name) {
			if moduleDefs[name] || (moduleName != "" && name == moduleName) {
				diags = append(diags, Diagnostic{
					Code:     "identifier-shadows-module-def",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"%q shadows a module-level definition (ETSI 5.2.2)",
						name),
					Node: dec,
					Span: syntax.SpanOf(dec),
				})
			}
		}
		top[name] = true
	}
	var walk func(n syntax.Node)
	walkValueDecl := func(vd *syntax.ValueDecl) {
		if vd == nil {
			return
		}
		for _, dec := range vd.Decls {
			if dec != nil && dec.Value != nil {
				walk(dec.Value)
			}
			declare(dec)
		}
	}
	walkBlock := func(b *syntax.BlockStmt) {
		if b == nil {
			return
		}
		push()
		for _, s := range b.Stmts {
			walk(s)
		}
		pop()
	}
	walk = func(n syntax.Node) {
		if n == nil {
			return
		}
		switch x := n.(type) {
		case *syntax.BlockStmt:
			walkBlock(x)
		case *syntax.DeclStmt:
			if vd, ok := x.Decl.(*syntax.ValueDecl); ok {
				walkValueDecl(vd)
			}
		case *syntax.ValueDecl:
			walkValueDecl(x)
		case *syntax.ForStmt:
			push()
			if x.Init != nil {
				walk(x.Init)
			}
			if x.Cond != nil {
				walk(x.Cond)
			}
			if x.Post != nil {
				walk(x.Post)
			}
			if x.Body != nil {
				walkBlock(x.Body)
			}
			pop()
		case *syntax.WhileStmt:
			push()
			if x.Cond != nil {
				walk(x.Cond)
			}
			if x.Body != nil {
				walkBlock(x.Body)
			}
			pop()
		case *syntax.DoWhileStmt:
			push()
			if x.Body != nil {
				walkBlock(x.Body)
			}
			if x.Cond != nil {
				walk(x.Cond)
			}
			pop()
		case *syntax.IfStmt:
			if x.Cond != nil {
				walk(x.Cond)
			}
			walk(x.Then)
			walk(x.Else)
		case *syntax.SelectStmt:
			if x.Tag != nil {
				walk(x.Tag)
			}
			for _, c := range x.Body {
				walk(c)
			}
		case *syntax.CaseClause:
			push()
			if x.Case != nil {
				walk(x.Case)
			}
			walk(x.Body)
			pop()
		case *syntax.AltStmt:
			walk(x.Body)
		case *syntax.CommClause:
			push()
			if x.X != nil {
				walk(x.X)
			}
			if x.Comm != nil {
				walk(x.Comm)
			}
			if x.Body != nil {
				walkBlock(x.Body)
			}
			pop()
		default:
			syntax.Inspect(n, func(in syntax.Node) bool {
				if in == n {
					return true
				}
				switch t := in.(type) {
				case *syntax.ForStmt, *syntax.WhileStmt, *syntax.DoWhileStmt,
					*syntax.IfStmt, *syntax.AltStmt, *syntax.SelectStmt,
					*syntax.BlockStmt, *syntax.CommClause, *syntax.CaseClause,
					*syntax.ValueDecl, *syntax.DeclStmt:
					_ = t
					walk(in)
					return false
				}
				return true
			})
		}
	}
	walkBlock(fn.Body)
	return diags
}

// collectModuleLevelNames returns the set of every top-level
// definition name in the module: consts, modulepars, templates,
// functions, testcases, signatures, types, etc. We use this to
// flag any body-local decl whose name collides.
func collectModuleLevelNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		// ValueDecl can carry multiple Declarators.
		if vd, ok := d.Def.(*syntax.ValueDecl); ok {
			for _, dec := range vd.Decls {
				if dec != nil && dec.Name != nil {
					out[dec.Name.String()] = true
				}
			}
			continue
		}
		if name := syntax.Name(d.Def); name != "" {
			out[name] = true
		}
	}
	return out
}

// collectAllComponentMemberNames returns the member-name set for
// each component type. We treat every var/const/timer/port
// declared inside the component body as a member; the resulting
// set is later flattened with the parents map by
// flattenComponentMembers.
func collectAllComponentMemberNames(mod *syntax.Module) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		ct, ok := d.Def.(*syntax.ComponentTypeDecl)
		if !ok || ct.Name == nil || ct.Body == nil {
			continue
		}
		members := map[string]bool{}
		for _, stmt := range ct.Body.Stmts {
			ds, ok := stmt.(*syntax.DeclStmt)
			if !ok {
				continue
			}
			if vd, ok := ds.Decl.(*syntax.ValueDecl); ok {
				for _, dec := range vd.Decls {
					if dec != nil && dec.Name != nil {
						members[dec.Name.String()] = true
					}
				}
			}
		}
		out[ct.Name.String()] = members
	}
	return out
}

// flattenComponentMembers folds inherited members from each
// component type's parents into its own member set, so a
// runs-on lookup sees the full effective scope.
func flattenComponentMembers(members map[string]map[string]bool, parents map[string][]string) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for ct := range members {
		merged := map[string]bool{}
		visited := map[string]bool{}
		var walk func(name string)
		walk = func(name string) {
			if visited[name] {
				return
			}
			visited[name] = true
			for _, p := range parents[name] {
				walk(p)
			}
			for k := range members[name] {
				merged[k] = true
			}
		}
		walk(ct)
		out[ct] = merged
	}
	return out
}

// collectFormalParamNames returns the set of formal parameter
// names declared on a FormalPars list. Used to flag body-local
// decls that shadow a parameter.
func collectFormalParamNames(fp *syntax.FormalPars) map[string]bool {
	out := map[string]bool{}
	if fp == nil {
		return out
	}
	for _, p := range fp.List {
		if p == nil || p.Name == nil {
			continue
		}
		out[p.Name.String()] = true
	}
	return out
}
