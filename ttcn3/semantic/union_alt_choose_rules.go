// union_alt_choose_rules.go enforces ETSI ES 201 873-1 clause
// 6.2.5.1 restriction on referencing union alternatives:
//
//   - When a union variable is assigned a composite value that
//     names one alternative (e.g. `v_u := { option1 := 1 }`) or a
//     dotted alternative assignment (`v_u.option1 := 1`), then
//     reading a different alternative (`v_u.option2`) on the RHS
//     of any expression shall cause an error.
//
// The walker mirrors union_alt_ref_rules: per-function body, we
// thread a chosen-alternative map across nested control flow and
// flag reads of any other alternative.
//
// We bound the check by:
//   - only watching variables whose declared type is a known
//     module-local `union` (so non-union fields are ignored);
//   - skipping arguments of introspection helpers (`ispresent`,
//     `ischosen`, etc.) the same way union_alt_ref does;
//   - clearing the chosen-alt mark when the entire variable is
//     reassigned to a non-composite value or to a different alt.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkUnionAltChooseRules(mod *syntax.Module) []Diagnostic {
	unions := collectUnionAlts(mod)
	if len(unions) == 0 {
		return nil
	}
	varTypes := collectModuleVarTypes(mod)
	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn == nil || fn.Body == nil {
			continue
		}
		bodyVarTypes := collectFuncBodyVarTypes(fn.Body)
		merged := map[string]string{}
		for k, v := range varTypes {
			merged[k] = v
		}
		for k, v := range bodyVarTypes {
			merged[k] = v
		}
		diags = append(diags, checkUnionAltChooseInBody(fn.Body, merged, unions)...)
	}
	return diags
}

func checkUnionAltChooseInBody(body *syntax.BlockStmt, varTypes map[string]string, unions map[string]map[string]bool) []Diagnostic {
	state := map[string]string{}
	if body == nil {
		return nil
	}
	return walkChooseBlock(body, state, varTypes, unions)
}

func walkChooseBlock(body *syntax.BlockStmt, state map[string]string, varTypes map[string]string, unions map[string]map[string]bool) []Diagnostic {
	if body == nil {
		return nil
	}
	var diags []Diagnostic
	for _, stmt := range body.Stmts {
		diags = append(diags, walkChooseStmt(stmt, state, varTypes, unions)...)
	}
	return diags
}

func walkChooseStmt(stmt syntax.Stmt, state map[string]string, varTypes map[string]string, unions map[string]map[string]bool) []Diagnostic {
	if stmt == nil {
		return nil
	}
	switch v := stmt.(type) {
	case *syntax.ExprStmt:
		return walkChooseExprStmt(v, state, varTypes, unions)
	case *syntax.DeclStmt:
		if v == nil || v.Decl == nil {
			return nil
		}
		vd, ok := v.Decl.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return nil
		}
		ty := identName(vd.Type)
		if ty == "" || !isUnionType(ty, unions) {
			return nil
		}
		var diags []Diagnostic
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil || d.Value == nil {
				continue
			}
			diags = append(diags, checkChooseReads(d.Value, state, varTypes, unions)...)
			alt := singleCompositeKey(d.Value)
			if alt != "" {
				state[d.Name.String()] = alt
			}
		}
		return diags
	case *syntax.IfStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, checkChooseReads(v.Cond, state, varTypes, unions)...)
		diags = append(diags, walkChooseBlock(v.Then, state, varTypes, unions)...)
		diags = append(diags, walkChooseStmt(v.Else, state, varTypes, unions)...)
		return diags
	case *syntax.BlockStmt:
		return walkChooseBlock(v, state, varTypes, unions)
	case *syntax.ForStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, walkChooseStmt(v.Init, state, varTypes, unions)...)
		diags = append(diags, checkChooseReads(v.Cond, state, varTypes, unions)...)
		diags = append(diags, walkChooseStmt(v.Post, state, varTypes, unions)...)
		diags = append(diags, walkChooseBlock(v.Body, state, varTypes, unions)...)
		return diags
	case *syntax.WhileStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, checkChooseReads(v.Cond, state, varTypes, unions)...)
		diags = append(diags, walkChooseBlock(v.Body, state, varTypes, unions)...)
		return diags
	case *syntax.DoWhileStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, walkChooseBlock(v.Body, state, varTypes, unions)...)
		diags = append(diags, checkChooseReads(v.Cond, state, varTypes, unions)...)
		return diags
	case *syntax.AltStmt:
		if v == nil {
			return nil
		}
		return walkChooseBlock(v.Body, state, varTypes, unions)
	case *syntax.SelectStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		if v.Tag != nil {
			diags = append(diags, checkChooseReads(v.Tag, state, varTypes, unions)...)
		}
		for _, cc := range v.Body {
			if cc == nil {
				continue
			}
			diags = append(diags, walkChooseBlock(cc.Body, state, varTypes, unions)...)
		}
		return diags
	}
	return nil
}

func walkChooseExprStmt(es *syntax.ExprStmt, state map[string]string, varTypes map[string]string, unions map[string]map[string]bool) []Diagnostic {
	if es == nil {
		return nil
	}
	be, ok := es.Expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return checkChooseReads(es.Expr, state, varTypes, unions)
	}
	diags := checkChooseReads(be.Y, state, varTypes, unions)
	updateChosenLHS(be.X, be.Y, state, varTypes, unions)
	return diags
}

// updateChosenLHS records the chosen alternative implied by an LHS
// reference shape. Supported shapes:
//
//   - `v_u := { altName := ... }`             -> state[v_u]=altName
//   - `v_u.altName := ...`                    -> state[v_u]=altName
//   - `v_u.altName.<deeper> := ...`           -> state[v_u]=altName
//     (only the top-level union's alternative is recorded; nested
//     unions are out of scope for this rule)
//
// Anything else either clears or leaves the chosen mark alone.
func updateChosenLHS(x, y syntax.Expr, state map[string]string, varTypes map[string]string, unions map[string]map[string]bool) {
	switch lhs := x.(type) {
	case *syntax.Ident:
		if lhs == nil {
			return
		}
		name := lhs.String()
		ty := varTypes[name]
		if !isUnionType(ty, unions) {
			return
		}
		if alt := singleCompositeKey(y); alt != "" {
			state[name] = alt
		} else {
			delete(state, name)
		}
	case *syntax.SelectorExpr:
		base, alt := topLevelUnionAccess(lhs, varTypes, unions)
		if base == "" || alt == "" {
			return
		}
		state[base] = alt
	}
}

// topLevelUnionAccess walks a SelectorExpr chain and returns the
// base identifier name and the immediate alternative name selected
// from it, provided the base variable's type is a known union and
// the alternative is one of its declared alts. Returns "","" for
// any shape that doesn't match.
func topLevelUnionAccess(sel *syntax.SelectorExpr, varTypes map[string]string, unions map[string]map[string]bool) (string, string) {
	if sel == nil || sel.X == nil || sel.Sel == nil {
		return "", ""
	}
	selId, ok := sel.Sel.(*syntax.Ident)
	if !ok || selId == nil {
		return "", ""
	}
	switch base := sel.X.(type) {
	case *syntax.Ident:
		if base == nil {
			return "", ""
		}
		ty := varTypes[base.String()]
		if !isUnionType(ty, unions) || !unions[ty][selId.String()] {
			return "", ""
		}
		return base.String(), selId.String()
	case *syntax.SelectorExpr:
		return topLevelUnionAccess(base, varTypes, unions)
	}
	return "", ""
}

func checkChooseReads(node syntax.Node, state map[string]string, varTypes map[string]string, unions map[string]map[string]bool) []Diagnostic {
	if node == nil {
		return nil
	}
	var diags []Diagnostic
	var walk func(syntax.Node)
	walk = func(n syntax.Node) {
		if n == nil {
			return
		}
		if ce, ok := n.(*syntax.CallExpr); ok && ce != nil {
			callee := identName(ce.Fun)
			switch callee {
			case "isbound", "ispresent", "ischosen", "isvalue",
				"lengthof", "sizeof", "match", "omit":
				return
			}
		}
		if sel, ok := n.(*syntax.SelectorExpr); ok && sel != nil {
			base, ok := sel.X.(*syntax.Ident)
			if ok && base != nil && sel.Sel != nil {
				if selId, ok := sel.Sel.(*syntax.Ident); ok && selId != nil {
					ty := varTypes[base.String()]
					if isUnionType(ty, unions) {
						chosen, has := state[base.String()]
						alt := selId.String()
						if has && unions[ty][alt] && alt != chosen {
							diags = append(diags, Diagnostic{
								Code:     "union-alt-not-chosen",
								Severity: SeverityError,
								Message: fmt.Sprintf(
									"reading union alternative %q on %q whose chosen alternative is %q (ETSI 6.2.5.1)",
									alt, base.String(), chosen),
								Node: sel,
								Span: syntax.SpanOf(sel),
							})
							return
						}
					}
				}
			}
		}
		n.Inspect(func(c syntax.Node) bool {
			if c == n {
				return true
			}
			walk(c)
			return false
		})
	}
	walk(node)
	return diags
}

// singleCompositeKey returns the name of the single field referenced
// in `{ altName := value }`. Returns "" for any other shape.
func singleCompositeKey(e syntax.Expr) string {
	cl, ok := e.(*syntax.CompositeLiteral)
	if !ok || cl == nil || len(cl.List) != 1 {
		return ""
	}
	be, ok := cl.List[0].(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return ""
	}
	return identName(be.X)
}

// collectUnionAlts returns a map of union-type-name to the set of
// alternative-field names. Other struct shapes are ignored.
func collectUnionAlts(mod *syntax.Module) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st == nil || st.Name == nil {
			continue
		}
		if st.KindTok == nil || st.KindTok.Kind() != syntax.UNION {
			continue
		}
		alts := map[string]bool{}
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			alts[f.Name.String()] = true
		}
		if len(alts) > 0 {
			out[st.Name.String()] = alts
		}
	}
	return out
}

func isUnionType(ty string, unions map[string]map[string]bool) bool {
	if ty == "" {
		return false
	}
	_, ok := unions[ty]
	return ok
}
