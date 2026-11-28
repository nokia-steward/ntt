// definition_uniqueness_rules.go extends the uniqueness of identifiers
// (ETSI ES 201 873-1 clause 5.2.2) to the definitions identifier_
// uniqueness.go does not see:
//
//   - formal parameters: unique in their list, and not of the name of a
//     module-level definition or of the module (5.2.2, 5.4, 16.3 a);
//   - component type members, ports among them: unique in the component
//     type definition (6.2.10.1, 9.2 a);
//   - enumerated values: their identifiers are reused only within other
//     structured type definitions, not for a module-level definition or a
//     local one (5.2.2, 6.2.4). Not in an edition 5 module, whose
//     enumerated labels have rules of their own (enum_v5_rules.go).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkDefinitionUniquenessRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	add := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	moduleDefs := collectModuleLevelNames(mod)
	moduleName := ""
	if mod.Name != nil {
		moduleName = mod.Name.String()
	}
	enumValues := map[string]string{}
	for _, d := range mod.Defs {
		et, ok := d.Def.(*syntax.EnumTypeDecl)
		if !ok || et.Name == nil || mod.Edition() == syntax.V5 {
			continue
		}
		for _, e := range et.Enums {
			if name := enumMemberName(e); name != "" {
				enumValues[name] = et.Name.String()
			}
		}
	}

	params := func(fp *syntax.FormalPars) {
		if fp == nil {
			return
		}
		seen := map[string]bool{}
		for _, p := range fp.List {
			if p == nil || p.Name == nil {
				continue
			}
			name := p.Name.String()
			switch {
			case seen[name]:
				add(p, "duplicate-identifier-in-scope", fmt.Sprintf("formal parameter %q is declared twice (ETSI 5.2.2, 5.4)", name))
			case moduleDefs[name] || name == moduleName:
				add(p, "identifier-shadows-module-def", fmt.Sprintf("formal parameter %q has the name of a module-level definition (ETSI 5.2.2)", name))
			case enumValues[name] != "":
				add(p, "enum-value-identifier-reused", fmt.Sprintf("formal parameter %q has the name of a value of enumerated type %s (ETSI 6.2.4)", name, enumValues[name]))
			}
			seen[name] = true
		}
	}

	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.FuncDecl:
			params(x.Params)
		case *syntax.TemplateDecl:
			params(x.Params)
		case *syntax.ComponentTypeDecl:
			if x.Body == nil {
				continue
			}
			seen := map[string]bool{}
			for _, stmt := range x.Body.Stmts {
				ds, ok := stmt.(*syntax.DeclStmt)
				if !ok {
					continue
				}
				vd, ok := ds.Decl.(*syntax.ValueDecl)
				if !ok {
					continue
				}
				for _, dec := range vd.Decls {
					if dec == nil || dec.Name == nil {
						continue
					}
					name := dec.Name.String()
					if seen[name] {
						add(dec, "duplicate-identifier-in-scope", fmt.Sprintf("%q is declared twice in component type %s (ETSI 6.2.10.1)", name, x.Name))
					}
					seen[name] = true
				}
			}
		}
	}

	// Definitions of the name of an enumerated value, module-level or
	// local.
	syntax.Inspect(mod, func(n syntax.Node) bool {
		var name *syntax.Ident
		switch x := n.(type) {
		case *syntax.Declarator:
			name = x.Name
		case *syntax.TemplateDecl:
			name = x.Name
		case *syntax.FuncDecl:
			name = x.Name
		case *syntax.StructTypeDecl, *syntax.EnumTypeDecl:
			// Fields and enumerated values of other types may reuse it.
			return false
		}
		if name != nil {
			if typ := enumValues[name.String()]; typ != "" {
				add(n, "enum-value-identifier-reused", fmt.Sprintf("%q is a value of enumerated type %s: its identifier is not reused for a definition (ETSI 6.2.4)", name, typ))
			}
		}
		return true
	})
	return diags
}
