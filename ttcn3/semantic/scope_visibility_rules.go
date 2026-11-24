// scope_visibility_rules.go reports a reference to a local definition from
// outside the scope unit that declares it (ETSI ES 201 873-1 clause 5.2):
// a variable, constant, template, timer or formal parameter of a function,
// altstep, test case or block is visible only there and in the scope units
// it contains, so a lower scope unit's definition is not visible in a
// higher one or a sibling (5.2.0), nor a variable of an if, select or for
// header outside the statement (19.2, 19.3).
//
// The rule fires only for an identifier that resolves to no definition and
// is the name of a local definition elsewhere in the module: an unknown
// name may be predefined, an operation keyword or imported, and is left to
// other rules.
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
	if len(locals) == 0 {
		return nil
	}
	// Module-level names hide nothing here: a name also declared at
	// module level resolves to that definition.
	var diags []Diagnostic
	skip := map[*syntax.Ident]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch x := n.(type) {
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
		case *syntax.Ident:
			name := x.String()
			if skip[x] || !locals[name] || len(m.tree.LookupWithDB(x, m.db)) > 0 {
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
