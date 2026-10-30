// timer_assign_compat_rules.go enforces a narrow flavour of
// ETSI ES 201 873-1 clause 6.3.6: a timer-typed variable
// can only be assigned a timer reference. The cleanest
// statically-checkable shape we flag is:
//
//   - the LHS is a bare identifier referring to a sibling
//     \`var timer name\` declared earlier in the same
//     function body,
//   - the RHS is a bare identifier referring to a sibling
//     \`var <non-timer-basic-type> other\` declared earlier
//     in the same function body.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkTimerAssignCompatRules(mod *syntax.Module) []Diagnostic {
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
		walkTimerAssignBlock(fn.Body, map[string]string{}, &diags)
	}
	return diags
}

func walkTimerAssignBlock(
	body *syntax.BlockStmt,
	scope map[string]string,
	diags *[]Diagnostic,
) {
	if body == nil {
		return
	}
	for _, st := range body.Stmts {
		if bs, ok := st.(*syntax.BlockStmt); ok && bs != nil {
			child := map[string]string{}
			for k, v := range scope {
				child[k] = v
			}
			walkTimerAssignBlock(bs, child, diags)
			continue
		}
		if ds, ok := st.(*syntax.DeclStmt); ok && ds != nil {
			vd, ok := ds.Decl.(*syntax.ValueDecl)
			if ok && vd != nil && vd.KindTok != nil &&
				vd.KindTok.Kind() == syntax.VAR &&
				vd.TemplateRestriction == nil {
				kind := varKindName(vd.Type)
				for _, dc := range vd.Decls {
					if dc == nil || dc.Name == nil {
						continue
					}
					if kind != "" {
						scope[dc.Name.String()] = kind
					}
					if dc.Value != nil && kind == "timer" {
						checkTimerAssignRHS(scope, dc.Name.String(), dc.Value, dc, diags)
					}
				}
				continue
			}
		}
		if es, ok := st.(*syntax.ExprStmt); ok && es != nil {
			if be, ok := es.Expr.(*syntax.BinaryExpr); ok &&
				be != nil && be.Op != nil &&
				be.Op.Kind() == syntax.ASSIGN {
				lhs, ok := be.X.(*syntax.Ident)
				if !ok || lhs == nil {
					continue
				}
				if scope[lhs.String()] != "timer" {
					continue
				}
				checkTimerAssignRHS(scope, lhs.String(), be.Y, be, diags)
			}
		}
	}
}

func varKindName(e syntax.Expr) string {
	id, ok := e.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return ""
	}
	if id.Tok.Kind() == syntax.TIMER {
		return "timer"
	}
	if name := basicTypeNameOf(e); name != "" {
		return name
	}
	if id.Tok.Kind() == syntax.IDENT {
		return "component-or-other:" + id.String()
	}
	return ""
}

func checkTimerAssignRHS(
	scope map[string]string,
	lhsName string,
	rhs syntax.Expr,
	node syntax.Node,
	diags *[]Diagnostic,
) {
	id, ok := rhs.(*syntax.Ident)
	if !ok || id == nil {
		return
	}
	got, ok := scope[id.String()]
	if !ok || got == "timer" {
		return
	}
	*diags = append(*diags, Diagnostic{
		Code:     "timer-assign-incompatible",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"cannot assign value %q of type %q to timer variable %q (ETSI 6.3.6)",
			id.String(), got, lhsName),
		Node: node,
		Span: syntax.SpanOf(node),
	})
}
