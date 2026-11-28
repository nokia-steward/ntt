// scope_visibility_rules.go reports a reference to a local definition from
// outside the scope unit that declares it (ETSI ES 201 873-1 clause 5.2):
// a variable, constant, template, timer or formal parameter of a function,
// altstep, test case or block is visible only there and in the scope units
// it contains, so a lower scope unit's definition is not visible in a
// higher one or a sibling (5.2.0), nor a variable of an if, select or for
// header outside the statement (19.2, 19.3).
//
// The field identifiers of a record, set or union are local to it (6.2.1,
// 6.2.5): a field name used on its own, outside a field reference, an
// assignment notation or a select union case, refers to nothing.
//
// A module's definitions are visible in another module only through an
// import statement, also when both are in the same file (8.2.3.1).
//
// The rule fires only for an identifier that resolves to no definition and
// is the name of a local definition or of a field elsewhere in the module,
// or of a definition of a module in the file that the module does not
// import at all: an unknown name may be predefined, an operation keyword
// or imported, and is left to other rules.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkScopeVisibilityRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}

	// The names of the module's local definitions.
	locals := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		fn, ok := n.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		if fn.Params != nil {
			for _, p := range fn.Params.List {
				if p != nil && p.Name != nil {
					locals[p.Name.String()] = true
				}
			}
		}
		if fn.Body == nil {
			return false
		}
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			switch x := n.(type) {
			case *syntax.Declarator:
				if x.Name != nil {
					locals[x.Name.String()] = true
				}
			case *syntax.TemplateDecl:
				if x.Name != nil {
					locals[x.Name.String()] = true
				}
			}
			return true
		})
		return false
	})
	// The field names of the module's records, sets and unions, but for
	// those that are also enumerated values in the file, which resolve by
	// the type they are compared with.
	fields := map[string]bool{}
	labels := map[string]bool{}
	syntax.Inspect(tree.Root, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.EnumTypeDecl:
			for _, e := range x.Enums {
				labels[enumLabelName(e)] = true
			}
		case *syntax.EnumSpec:
			for _, e := range x.Enums {
				labels[enumLabelName(e)] = true
			}
		}
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.SubTypeDecl:
			// Its field is the name of the type.
			if x.Field != nil {
				syntax.Inspect(x.Field.Type, func(n syntax.Node) bool {
					if f, ok := n.(*syntax.Field); ok && f.Name != nil {
						fields[f.Name.String()] = true
					}
					return true
				})
			}
			return false
		case *syntax.Field:
			if x.Name != nil {
				fields[x.Name.String()] = true
			}
		}
		return true
	})
	for l := range labels {
		delete(fields, l)
	}
	// The definitions of the file's other modules, by name, that the
	// module imports nothing from, directly or through the imports of a
	// module it imports (8.2.3.7).
	modules := map[string]*syntax.Module{}
	for _, mn := range tree.Modules() {
		if m, ok := mn.Node.(*syntax.Module); ok {
			modules[syntax.Name(m.Name)] = m
		}
	}
	imported := map[string]bool{}
	for q := []*syntax.Module{mod}; len(q) > 0; q = q[1:] {
		if imported[syntax.Name(q[0].Name)] {
			continue
		}
		imported[syntax.Name(q[0].Name)] = true
		q[0].Inspect(func(n syntax.Node) bool {
			if imp, ok := n.(*syntax.ImportDecl); ok {
				name := syntax.Name(imp.Module)
				if m := modules[name]; m != nil {
					q = append(q, m)
				}
				imported[name] = imported[name] || modules[name] == nil
				return false
			}
			return true
		})
	}
	foreign := map[string]string{}
	for name, m := range modules {
		if !imported[name] {
			for _, def := range moduleLevelNames(m.Defs) {
				if !labels[def] {
					foreign[def] = name
				}
			}
		}
	}
	if len(locals) == 0 && len(fields) == 0 && len(foreign) == 0 {
		return nil
	}
	// Module-level names hide nothing here: a name also declared at
	// module level resolves to that definition.
	var diags []Diagnostic
	skip := map[*syntax.Ident]bool{}
	// Identifiers naming the fields of the received value or the
	// parameters of a signature in a redirect, `-> value (v := f)`.
	redirected := map[*syntax.Ident]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
		case *syntax.RedirectExpr:
			var list []syntax.Expr
			for _, e := range append(append([]syntax.Expr(nil), x.Value...), x.Param...) {
				if p, ok := e.(*syntax.ParenExpr); ok {
					list = append(list, p.List...)
				} else {
					list = append(list, e)
				}
			}
			for _, e := range list {
				if be, ok := e.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
					syntax.Inspect(be.Y, func(n syntax.Node) bool {
						if id, ok := n.(*syntax.Ident); ok {
							redirected[id] = true
						}
						return true
					})
				}
			}
		case *syntax.SelectorExpr:
			// A field, alternative or member name.
			if id, ok := x.Sel.(*syntax.Ident); ok {
				skip[id] = true
			}
		case *syntax.BinaryExpr:
			// A field or named parameter of an assignment notation, and a
			// port or a component of a configuration operation's c:p.
			if x.Op != nil && (x.Op.Kind() == syntax.ASSIGN || x.Op.Kind() == syntax.COLON) {
				if id, ok := x.X.(*syntax.Ident); ok && x.Op.Kind() == syntax.ASSIGN {
					if _, isAssign := tree.ParentOf(x).(*syntax.ExprStmt); !isAssign {
						skip[id] = true
					}
				}
				if id, ok := x.Y.(*syntax.Ident); ok && x.Op.Kind() == syntax.COLON {
					skip[id] = true
				}
			}
		case *syntax.SelectStmt:
			// The alternatives of a select union's cases.
			if x.Union != nil {
				for _, c := range x.Body {
					if c.Case == nil {
						continue
					}
					for _, e := range c.Case.List {
						if id, ok := e.(*syntax.Ident); ok {
							skip[id] = true
						}
					}
				}
			}
		case *syntax.Declarator:
			if x.Name != nil {
				skip[x.Name] = true
			}
		case *syntax.FormalPar:
			if x.Name != nil {
				skip[x.Name] = true
			}
		case *syntax.Field:
			if x.Name != nil {
				skip[x.Name] = true
			}
		case *syntax.WithSpec:
			// Attribute qualifiers name fields.
			return false
		case *syntax.EnumTypeDecl, *syntax.EnumSpec:
			return false
		case *syntax.Ident:
			name := x.String()
			if skip[x] || !locals[name] && !fields[name] && foreign[name] == "" || len(m.tree.LookupWithDB(x, m.db)) > 0 {
				return true
			}
			if from := foreign[name]; from != "" && !locals[name] {
				diags = append(diags, Diagnostic{
					Code:     "definition-not-imported",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s is a definition of module %s, which %s does not import (ETSI 8.2.3.1)", name, from, syntax.Name(mod.Name)),
					Node:     x,
					Span:     syntax.SpanOf(x),
				})
				return true
			}
			if !locals[name] {
				if redirected[x] {
					return true
				}
				diags = append(diags, Diagnostic{
					Code:     "field-name-outside-type",
					Severity: SeverityError,
					Message:  fmt.Sprintf("%s is a field name, local to the type that declares it, and refers to nothing here (ETSI 6.2.1)", name),
					Node:     x,
					Span:     syntax.SpanOf(x),
				})
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "local-definition-not-visible",
				Severity: SeverityError,
				Message:  fmt.Sprintf("%s is declared in another scope unit and is not visible here (ETSI 5.2)", name),
				Node:     x,
				Span:     syntax.SpanOf(x),
			})
		}
		return true
	})
	return diags
}

// moduleLevelNames returns the names of the module-level definitions defs
// declare, in groups too.
func moduleLevelNames(defs []*syntax.ModuleDef) []string {
	var names []string
	for _, d := range defs {
		if d == nil || d.Def == nil {
			continue
		}
		switch x := d.Def.(type) {
		case *syntax.GroupDecl:
			names = append(names, moduleLevelNames(x.Defs)...)
		case *syntax.ImportDecl, *syntax.FriendDecl, *syntax.ControlPart:
		default:
			names = append(names, moduleDefNames(x)...)
		}
	}
	return names
}
