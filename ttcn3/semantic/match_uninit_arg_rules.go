// match_uninit_arg_rules.go enforces ETSI ES 201 873-1
// clause 15.9: the operands of the \`match\` predefined
// operation must be completely initialised. We only handle
// the cleanest shape:
//
//   - the call \`match(X, ...)\` appears anywhere inside a
//     function body,
//   - X is a bare identifier referring to a sibling local
//     \`var T name\` declared without an initialiser,
//   - that variable is never assigned anywhere inside the
//     function body before the call site.
//
// Anything more elaborate (conditional assignment, member
// access, formal parameters, separate functions) is ignored.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkMatchUninitArgRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		declared := declaredUninitLocals(fn.Body)
		if len(declared) == 0 {
			continue
		}
		uses := countIdentUses(fn.Body)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ce, ok := n.(*syntax.CallExpr)
			if !ok || ce == nil || ce.Args == nil {
				return true
			}
			id, ok := ce.Fun.(*syntax.Ident)
			if !ok || id == nil || id.String() != "match" {
				return true
			}
			if len(ce.Args.List) == 0 {
				return true
			}
			argID, ok := ce.Args.List[0].(*syntax.Ident)
			if !ok || argID == nil {
				return true
			}
			name := argID.String()
			if !declared[name] {
				return true
			}
			if uses[name] != 1 {
				return true
			}
			diags = append(diags, Diagnostic{
				Code:     "match-uninit-arg",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"the first operand of `match` is variable %q which is never initialised; both operands must be completely initialised (ETSI 15.9 b)",
					name),
				Node: ce,
				Span: syntax.SpanOf(ce),
			})
			return true
		})
	}
	return diags
}

func declaredUninitLocals(body *syntax.BlockStmt) map[string]bool {
	if body == nil {
		return nil
	}
	out := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			return true
		}
		if vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil || dc.Value != nil {
				continue
			}
			out[dc.Name.String()] = true
		}
		return true
	})
	return out
}

func countIdentUses(body *syntax.BlockStmt) map[string]int {
	out := map[string]int{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		id, ok := n.(*syntax.Ident)
		if !ok || id == nil || id.Tok == nil {
			return true
		}
		if id.Tok.Kind() != syntax.IDENT {
			return true
		}
		if id.IsName {
			return true
		}
		out[id.String()]++
		return true
	})
	return out
}

func uninitLocalsNeverAssigned(body *syntax.BlockStmt) map[string]bool {
	if body == nil {
		return nil
	}
	declaredUninit := map[string]bool{}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil {
			return true
		}
		if vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil || dc.Value != nil {
				continue
			}
			declaredUninit[dc.Name.String()] = true
		}
		return true
	})
	if len(declaredUninit) == 0 {
		return nil
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		if be, ok := n.(*syntax.BinaryExpr); ok && be != nil &&
			be.Op != nil && be.Op.Kind() == syntax.ASSIGN {
			if id, ok := be.X.(*syntax.Ident); ok && id != nil {
				delete(declaredUninit, id.String())
			}
			if id := selectorBaseIdent(be.X); id != "" {
				delete(declaredUninit, id)
			}
		}
		if ce, ok := n.(*syntax.CallExpr); ok && ce != nil && ce.Args != nil {
			fnID, _ := ce.Fun.(*syntax.Ident)
			fnName := ""
			if fnID != nil {
				fnName = fnID.String()
			}
			for _, arg := range ce.Args.List {
				id, ok := arg.(*syntax.Ident)
				if !ok || id == nil {
					continue
				}
				if fnName == "match" {
					continue
				}
				delete(declaredUninit, id.String())
			}
		}
		return true
	})
	return declaredUninit
}

func selectorBaseIdent(e syntax.Expr) string {
	se, ok := e.(*syntax.SelectorExpr)
	if !ok || se == nil {
		return ""
	}
	base, ok := se.X.(*syntax.Ident)
	if !ok || base == nil {
		return ""
	}
	return base.String()
}
