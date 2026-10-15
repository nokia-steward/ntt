// index_redirect_dim_rules.go enforces ETSI ES 201 873-1 clauses
// 22.{2,3}.x restrictions i / j on the `@index value v` redirect
// target's array depth versus the port-array's dimension count:
//
//   - Restriction i (1-D port arrays): the target shall be a
//     scalar integer-typed variable.  Passing an array (e.g.
//     `var integer v[1]`) is an error.
//   - Restriction j (multi-D port arrays): the target shall be
//     an integer array whose dimension count equals the port-
//     array's dimension count.
//
// We only catch the static shapes where:
//   - the receiver (`p` in `any from p.<op>(...) -> @index value
//     v`) is a known port-array instance on the enclosing
//     component (or one of its parents);
//   - the target var `v` was declared in the function body or as
//     a member of the enclosing component, with a known integer
//     type and a recognisable array shape.
//
// The check is intentionally conservative: anything we cannot
// resolve (parameterised arrays, cross-module ports, computed
// dim counts) is left untouched.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkIndexRedirectDimRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}

	compPortInfo := collectComponentPortInfo(mod)
	parents := collectComponentParents(mod)
	compPortInfo = flattenComponentPortInfo(compPortInfo, parents)

	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
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
		ports := compPortInfo[comp]
		if ports == nil {
			continue
		}
		localDims := collectFuncBodyArrayDims(fn.Body)
		localDimLens := collectFuncBodyArrayDimLens(fn.Body)
		localVarTypes := collectFuncBodyVarTypes(fn.Body)

		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			r, ok := n.(*syntax.RedirectExpr)
			if !ok || r == nil || r.IndexTok == nil || r.Index == nil {
				return true
			}
			port := indexRedirectPort(r.X)
			if port == "" {
				return true
			}
			info, isPort := ports[port]
			if !isPort || info.dims == 0 {
				return true
			}
			target := identName(r.Index)
			if target == "" {
				return true
			}
			tdims, has := localDims[target]
			if !has {
				return true
			}
			if tdims == 0 {
				if ty, ok := localVarTypes[target]; ok && ty != "" &&
					isIntegerLikeTypeWithMod(ty, mod) && !isIntegerLikeType(ty) {
					tdims = 1
				}
			}
			switch {
			case info.dims == 1 && tdims > 0:
				diags = append(diags, Diagnostic{
					Code:     "index-redirect-1d-port-needs-scalar",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"@index value %s: port `%s` is 1-D; target must be a scalar integer (ETSI 22.{2,3} restriction i)",
						target, port),
					Node: r,
					Span: syntax.SpanOf(r),
				})
			case info.dims > 1 && tdims == 0:
				diags = append(diags, Diagnostic{
					Code:     "index-redirect-multid-needs-array",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"@index value %s: port `%s` is %d-D; target must be an array of integer (ETSI 22.{2,3} restriction j)",
						target, port, info.dims),
					Node: r,
					Span: syntax.SpanOf(r),
				})
			case info.dims > 1 && tdims > 1:
				diags = append(diags, Diagnostic{
					Code:     "index-redirect-multid-needs-1d-target",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"@index value %s: target is a %d-D array but multi-dimensional port arrays require a single-dimensional integer array (ETSI 22.{2,3} restriction j)",
						target, tdims),
					Node: r,
					Span: syntax.SpanOf(r),
				})
			case info.dims > 1 && tdims == 1:
				if tlen, ok := localDimLens[target]; ok && tlen > 0 && tlen != info.dims {
					diags = append(diags, Diagnostic{
						Code:     "index-redirect-length-mismatch",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"@index value %s: array length %d does not match port `%s` dimension count %d (ETSI 22.{2,3} restriction j)",
							target, tlen, port, info.dims),
						Node: r,
						Span: syntax.SpanOf(r),
					})
				}
			}
			return true
		})
	}
	return diags
}

// indexRedirectPort returns the leading port identifier name in
// the receive-shape expression of a RedirectExpr (e.g. `p` in
// `any from p.getcall(...)` or `p.receive(...)`). Returns ""
// for anything more complex than `<from> <Ident>.<op>(...)`.
func indexRedirectPort(x syntax.Expr) string {
	if from, ok := x.(*syntax.FromExpr); ok && from != nil {
		x = from.X
	}
	var receiver syntax.Expr
	switch v := x.(type) {
	case *syntax.CallExpr:
		if sel, ok := v.Fun.(*syntax.SelectorExpr); ok && sel != nil {
			receiver = sel.X
		}
	case *syntax.SelectorExpr:
		if v != nil {
			receiver = v.X
		}
	}
	if receiver == nil {
		return ""
	}
	if from, ok := receiver.(*syntax.FromExpr); ok && from != nil {
		receiver = from.X
	}
	return identName(receiver)
}

// collectFuncBodyArrayDims returns name -> array-dim-count for
// every `var T name[d1][d2]...` declaration inside body. The
// resulting count is 0 for scalar declarations and the number of
// `[...]` brackets otherwise.
func collectFuncBodyArrayDims(body syntax.Node) map[string]int {
	out := map[string]int{}
	if body == nil {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = len(dec.ArrayDef)
		}
		return true
	})
	return out
}

// collectFuncBodyArrayDimLens returns name -> first-array-dim's
// integer-literal length for every `var T name[N]` decl, when N
// is a plain integer literal. Variable-length forms (`name[K]`
// with K an identifier or expression) are omitted from the map.
func collectFuncBodyArrayDimLens(body syntax.Node) map[string]int {
	out := map[string]int{}
	if body == nil {
		return out
	}
	syntax.Inspect(body, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil ||
				len(dec.ArrayDef) == 0 {
				continue
			}
			pe := dec.ArrayDef[0]
			if pe == nil || len(pe.List) == 0 {
				continue
			}
			if lit, ok := pe.List[0].(*syntax.ValueLiteral); ok &&
				lit != nil && lit.Tok != nil {
				var n int
				if _, err := fmt.Sscanf(lit.Tok.String(), "%d", &n); err == nil && n > 0 {
					out[dec.Name.String()] = n
				}
			}
		}
		return true
	})
	return out
}
