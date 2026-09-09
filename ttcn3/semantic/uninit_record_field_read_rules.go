// uninit_record_field_read_rules.go enforces ETSI ES 201 873-1
// clause 7: "at the point when an expression is evaluated, the
// evaluated values of the operands used in expressions shall be
// completely initialized". We catch the narrow shape where a
// local record-typed variable is declared without an initializer
// AND only some of its fields are subsequently assigned AND an
// expression then reads one of the unassigned fields.
//
// Concretely, for every function body we walk the statements in
// program order and track:
//
//   - declared-uninitialized record-typed locals;
//   - subsequent whole-value assignments (v := ...) that wipe
//     the uninitialized tag;
//   - subsequent dot-assignments (v.field := ...) that mark the
//     field as set.
//
// We then report any expression reference to v.field that:
//   - is NOT the LHS of an assignment;
//   - belongs to a variable still flagged as uninitialized;
//   - and points to a field that was never marked set.
//
// This is a deliberately narrow check. We only inspect record
// types we resolved from module-level decls. Anything we cannot
// resolve falls through silently.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkUninitRecordFieldReadRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	records := collectRecordFieldTypes(mod)
	if len(records) == 0 {
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
		diags = append(diags, walkUninitRecord(fn.Body, records)...)
	}
	return diags
}

type uninitRecordCtx struct {
	uninit   map[string]string          // var -> recordTypeName
	setField map[string]map[string]bool // var -> field name -> set?
}

func newUninitRecordCtx() *uninitRecordCtx {
	return &uninitRecordCtx{
		uninit:   map[string]string{},
		setField: map[string]map[string]bool{},
	}
}

func walkUninitRecord(body *syntax.BlockStmt, records map[string][]recordFieldFull) []Diagnostic {
	if body == nil {
		return nil
	}
	ctx := newUninitRecordCtx()
	return walkUninitRecordStmts(body.Stmts, ctx, records)
}

func walkUninitRecordStmts(stmts []syntax.Stmt, ctx *uninitRecordCtx, records map[string][]recordFieldFull) []Diagnostic {
	var diags []Diagnostic
	for _, st := range stmts {
		if st == nil {
			continue
		}
		// Track declarations and assignments before scanning
		// expressions of this statement so we honour program
		// order.
		updateUninitCtxBeforeStmt(st, ctx, records)
		diags = append(diags, scanStmtForUninitReads(st, ctx, records)...)
		updateUninitCtxAfterStmt(st, ctx, records)
	}
	return diags
}

func updateUninitCtxBeforeStmt(st syntax.Stmt, ctx *uninitRecordCtx, records map[string][]recordFieldFull) {
	ds, ok := st.(*syntax.DeclStmt)
	if !ok || ds == nil {
		return
	}
	vd, ok := ds.Decl.(*syntax.ValueDecl)
	if !ok || vd == nil || vd.KindTok == nil ||
		vd.KindTok.Kind() != syntax.VAR {
		return
	}
	ty := identName(vd.Type)
	if _, ok := records[ty]; !ok {
		return
	}
	for _, dec := range vd.Decls {
		if dec == nil || dec.Name == nil {
			continue
		}
		name := dec.Name.String()
		if dec.Value != nil {
			// Has initializer: not tracked.
			delete(ctx.uninit, name)
			delete(ctx.setField, name)
			continue
		}
		ctx.uninit[name] = ty
		ctx.setField[name] = map[string]bool{}
	}
}

func updateUninitCtxAfterStmt(st syntax.Stmt, ctx *uninitRecordCtx, records map[string][]recordFieldFull) {
	syntax.Inspect(st, func(n syntax.Node) bool {
		re, ok := n.(*syntax.RedirectExpr)
		if ok && re != nil {
			clearRedirectValueTargets(re, ctx)
		}
		return true
	})
	es, ok := st.(*syntax.ExprStmt)
	if !ok || es == nil {
		return
	}
	be, ok := es.Expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil ||
		be.Op.Kind() != syntax.ASSIGN {
		return
	}
	switch lhs := be.X.(type) {
	case *syntax.Ident:
		name := lhs.String()
		if _, hit := ctx.uninit[name]; hit {
			delete(ctx.uninit, name)
		}
	case *syntax.SelectorExpr:
		root, firstField := dotChainRootField(lhs)
		if root == "" || firstField == "" {
			return
		}
		if _, hit := ctx.uninit[root]; !hit {
			return
		}
		if ctx.setField[root] == nil {
			ctx.setField[root] = map[string]bool{}
		}
		ctx.setField[root][firstField] = true
	case *syntax.IndexExpr:
		// v[idx] := ... - find the underlying root ident.
		root := indexChainRoot(lhs)
		if root == "" {
			return
		}
		if _, hit := ctx.uninit[root]; hit {
			delete(ctx.uninit, root)
		}
	}
}

func clearRedirectValueTargets(re *syntax.RedirectExpr, ctx *uninitRecordCtx) {
	for _, target := range re.Value {
		if name := rootIdentName(target); name != "" {
			delete(ctx.uninit, name)
			delete(ctx.setField, name)
		}
	}
}

// dotChainRootField unwinds a SelectorExpr/IndexExpr chain
//
//	v.a.b.c -> root="v", firstField="a"
//	v.a[0].b -> root="v", firstField="a"
//
// It returns ("","") for any non-dot chain.
func dotChainRootField(sel *syntax.SelectorExpr) (string, string) {
	for sel != nil {
		switch x := sel.X.(type) {
		case *syntax.Ident:
			id, ok := sel.Sel.(*syntax.Ident)
			if !ok || id == nil {
				return "", ""
			}
			return x.String(), id.String()
		case *syntax.SelectorExpr:
			sel = x
		case *syntax.IndexExpr:
			inner := unwrapIndex(x)
			if inner == nil {
				return "", ""
			}
			sel = inner
		default:
			return "", ""
		}
	}
	return "", ""
}

// unwrapIndex peels off chained IndexExprs to expose the
// underlying SelectorExpr, if any.
func unwrapIndex(ie *syntax.IndexExpr) *syntax.SelectorExpr {
	for ie != nil {
		switch x := ie.X.(type) {
		case *syntax.SelectorExpr:
			return x
		case *syntax.IndexExpr:
			ie = x
		default:
			return nil
		}
	}
	return nil
}

func indexChainRoot(ie *syntax.IndexExpr) string {
	switch x := ie.X.(type) {
	case *syntax.Ident:
		return x.String()
	case *syntax.SelectorExpr:
		root, _ := dotChainRootField(x)
		return root
	case *syntax.IndexExpr:
		return indexChainRoot(x)
	}
	return ""
}

// uninitReadSafePredicates is the closed set of predefined
// functions whose argument may legitimately be uninitialized:
// isbound / ispresent / isvalue / ischosen.
var uninitReadSafePredicates = map[string]bool{
	"isbound":   true,
	"ispresent": true,
	"isvalue":   true,
	"ischosen":  true,
	"isunbound": true,
}

func scanStmtForUninitReads(st syntax.Stmt, ctx *uninitRecordCtx, records map[string][]recordFieldFull) []Diagnostic {
	var diags []Diagnostic
	// First, find every selector inside an uninit-safe predicate
	// (isbound / ispresent / ...) and add it to a pointer-identity
	// skipset. Also skip selectors that appear as the LHS of an
	// assignment - those are sets, not reads.
	skip := map[*syntax.SelectorExpr]bool{}
	syntax.Inspect(st, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		fnID, ok := ce.Fun.(*syntax.Ident)
		if !ok || fnID == nil {
			return true
		}
		if !uninitReadSafePredicates[fnID.String()] {
			return true
		}
		if ce.Args == nil {
			return true
		}
		for _, arg := range ce.Args.List {
			syntax.Inspect(arg, func(m syntax.Node) bool {
				if sel, ok := m.(*syntax.SelectorExpr); ok && sel != nil {
					skip[sel] = true
				}
				return true
			})
		}
		return true
	})
	syntax.Inspect(st, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil ||
			be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		syntax.Inspect(be.X, func(m syntax.Node) bool {
			if s, ok := m.(*syntax.SelectorExpr); ok && s != nil {
				skip[s] = true
			}
			return true
		})
		return true
	})
	// A `-> value v` redirect assigns v as part of the matching, before the
	// clause body runs, so reads of v inside THAT body see an initialized
	// value. The context clear in updateUninitCtxAfterStmt happens only once
	// the whole statement is scanned, which is too late for an alt whose
	// redirect and read live in the same statement — the ordinary
	// `alt { [] p.receive(R:?) -> value r { log(r.body) } }`. Scope the
	// exemption to the clause carrying the redirect, so a sibling branch
	// that has no redirect is still checked.
	syntax.Inspect(st, func(n syntax.Node) bool {
		cc, ok := n.(*syntax.CommClause)
		if !ok || cc == nil || cc.Body == nil || cc.Comm == nil {
			return true
		}
		redirected := map[string]bool{}
		syntax.Inspect(cc.Comm, func(m syntax.Node) bool {
			re, ok := m.(*syntax.RedirectExpr)
			if !ok || re == nil {
				return true
			}
			for _, target := range re.Value {
				if name := rootIdentName(target); name != "" {
					redirected[name] = true
				}
			}
			return true
		})
		if len(redirected) == 0 {
			return true
		}
		syntax.Inspect(cc.Body, func(m syntax.Node) bool {
			sel, ok := m.(*syntax.SelectorExpr)
			if !ok || sel == nil {
				return true
			}
			if root, ok := sel.X.(*syntax.Ident); ok && root != nil && redirected[root.String()] {
				skip[sel] = true
			}
			return true
		})
		return true
	})
	syntax.Inspect(st, func(n syntax.Node) bool {
		sel, ok := n.(*syntax.SelectorExpr)
		if !ok || sel == nil {
			return true
		}
		if skip[sel] {
			return true
		}
		root, ok := sel.X.(*syntax.Ident)
		if !ok || root == nil {
			return true
		}
		fld, ok := sel.Sel.(*syntax.Ident)
		if !ok || fld == nil {
			return true
		}
		varName := root.String()
		ty, isTracked := ctx.uninit[varName]
		if !isTracked {
			return true
		}
		fields := records[ty]
		if fields == nil {
			return true
		}
		fname := fld.String()
		found := false
		for _, f := range fields {
			if f.name == fname {
				found = true
				break
			}
		}
		if !found {
			return true
		}
		if ctx.setField[varName] != nil && ctx.setField[varName][fname] {
			return true
		}
		diags = append(diags, Diagnostic{
			Code:     "uninit-record-field-read",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"reading uninitialized field %q of locally-declared record-typed var %q (ETSI 7)",
				fname, varName),
			Node: sel,
			Span: syntax.SpanOf(sel),
		})
		return true
	})
	return diags
}
