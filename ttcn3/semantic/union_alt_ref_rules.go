// union_alt_ref_rules.go enforces ETSI ES 201 873-1 clause 15.6.5
// "Referencing union alternatives" inside template variables:
//
//   - Restriction a) Referencing an alternative of a union template
//     field to which `omit`, `*` (AnyValueOrNone), a parenthesised
//     value list, or `complement(...)` has been assigned shall be
//     rejected.
//   - Restriction c) Referencing an alternative of a union template
//     field to which the `ifpresent` attribute is attached shall be
//     rejected.
//   - Restriction d) Referencing an alternative of an address-typed
//     union template field whose value is `null` shall be rejected.
//
// The walker tracks per-block, per-path assignment state for every
// template variable: when an assignment binds a "no-alternative"
// pattern to a path, every subsequent read of the same path or a
// deeper child is flagged.
//
// The rule is intentionally conservative: it only watches assignments
// inside the same function / testcase body and only fires when a
// concrete bad-pattern shape (omit / `*` / list / complement /
// ifpresent / null) is detected. Plain values (`?`, single matching
// values, composite literals) clear the bad mark.
package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

type unionAltBadKind int

const (
	unionAltOK unionAltBadKind = iota
	unionAltOmit
	unionAltAnyOrNone
	unionAltValueList
	unionAltComplement
	unionAltIfPresent
	unionAltNull
)

// isValueShapeBad reports whether the bad-kind makes the path's
// value itself a non-extractable matching shape - i.e. reading the
// exact same path yields something that cannot be assigned to a
// regular variable / template of the alternative's type.
func isValueShapeBad(k unionAltBadKind) bool {
	switch k {
	case unionAltValueList, unionAltComplement, unionAltNull:
		return true
	}
	return false
}

func (k unionAltBadKind) describe() string {
	switch k {
	case unionAltOmit:
		return "omit"
	case unionAltAnyOrNone:
		return "AnyValueOrNone (`*`)"
	case unionAltValueList:
		return "a value list"
	case unionAltComplement:
		return "a complemented list"
	case unionAltIfPresent:
		return "an `ifpresent`-attributed value"
	case unionAltNull:
		return "`null`"
	}
	return ""
}

func (a *Analyzer) checkUnionAltRefRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		var body *syntax.BlockStmt
		switch v := n.(type) {
		case *syntax.FuncDecl:
			if v != nil {
				body = v.Body
			}
		}
		if body == nil {
			return true
		}
		diags = append(diags, checkUnionAltInBody(body)...)
		return true
	})
	return diags
}

func checkUnionAltInBody(body *syntax.BlockStmt) []Diagnostic {
	state := map[string]unionAltBadKind{}
	return walkUnionAltBlock(body, state)
}

// walkUnionAltBlock walks a block in source order, threading a shared
// state map across nested if / for / while / alt / select bodies so
// bad-path marks survive into deeper reads. The state map is mutated
// in place; callers must copy it themselves if they want a snapshot.
func walkUnionAltBlock(body *syntax.BlockStmt, state map[string]unionAltBadKind) []Diagnostic {
	var diags []Diagnostic
	if body == nil {
		return diags
	}
	for _, stmt := range body.Stmts {
		diags = append(diags, walkUnionAltStmt(stmt, state)...)
	}
	return diags
}

func walkUnionAltStmt(stmt syntax.Stmt, state map[string]unionAltBadKind) []Diagnostic {
	if stmt == nil {
		return nil
	}
	switch v := stmt.(type) {
	case *syntax.ExprStmt:
		if v == nil {
			return nil
		}
		return walkUnionAltExprStmt(v, state)
	case *syntax.IfStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, checkUnionAltReads(v.Cond, state)...)
		diags = append(diags, walkUnionAltBlock(v.Then, state)...)
		diags = append(diags, walkUnionAltStmt(v.Else, state)...)
		return diags
	case *syntax.BlockStmt:
		return walkUnionAltBlock(v, state)
	case *syntax.ForStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, walkUnionAltStmt(v.Init, state)...)
		diags = append(diags, checkUnionAltReads(v.Cond, state)...)
		diags = append(diags, walkUnionAltStmt(v.Post, state)...)
		diags = append(diags, walkUnionAltBlock(v.Body, state)...)
		return diags
	case *syntax.ForRangeStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, walkUnionAltStmt(v.Init, state)...)
		diags = append(diags, checkUnionAltReads(v.Range, state)...)
		diags = append(diags, walkUnionAltBlock(v.Body, state)...)
		return diags
	case *syntax.WhileStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, checkUnionAltReads(v.Cond, state)...)
		diags = append(diags, walkUnionAltBlock(v.Body, state)...)
		return diags
	case *syntax.DoWhileStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		diags = append(diags, walkUnionAltBlock(v.Body, state)...)
		diags = append(diags, checkUnionAltReads(v.Cond, state)...)
		return diags
	case *syntax.AltStmt:
		if v == nil {
			return nil
		}
		return walkUnionAltBlock(v.Body, state)
	case *syntax.SelectStmt:
		if v == nil {
			return nil
		}
		var diags []Diagnostic
		if v.Tag != nil {
			diags = append(diags, checkUnionAltReads(v.Tag, state)...)
		}
		// CaseClause bodies are blocks of statements; recurse.
		for _, cc := range v.Body {
			if cc == nil {
				continue
			}
			diags = append(diags, walkUnionAltBlock(cc.Body, state)...)
		}
		return diags
	}
	return nil
}

func walkUnionAltExprStmt(es *syntax.ExprStmt, state map[string]unionAltBadKind) []Diagnostic {
	be, ok := es.Expr.(*syntax.BinaryExpr)
	if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
		return checkUnionAltReads(es.Expr, state)
	}
	diags := checkUnionAltReads(be.Y, state)
	lhs := selectorPath(be.X)
	if lhs == "" {
		return diags
	}
	kind := classifyUnionAltRHS(be.Y)
	state[lhs] = kind
	// Narrowing assignments clear any bad marks on deeper
	// children (state["lhs.X"]). They also clear bad marks on
	// strict ancestors because TTCN-3 6.2.1.1 says an LHS
	// reference recursively expands omitted intermediate fields.
	for p := range state {
		if strings.HasPrefix(p, lhs+".") {
			delete(state, p)
		}
		if strings.HasPrefix(lhs+".", p+".") {
			delete(state, p)
		}
	}
	if kind != unionAltOK {
		state[lhs] = kind
	}
	return diags
}

func checkUnionAltReads(expr syntax.Node, state map[string]unionAltBadKind) []Diagnostic {
	if expr == nil {
		return nil
	}
	var diags []Diagnostic
	var walk func(n syntax.Node)
	walk = func(n syntax.Node) {
		if n == nil {
			return
		}
		// Introspection / matching helpers don't dereference the
		// alternative; skip their arguments to avoid false
		// positives.
		if ce, ok := n.(*syntax.CallExpr); ok && ce != nil {
			callee := identName(ce.Fun)
			switch callee {
			case "isbound", "ispresent", "ischosen", "isvalue",
				"lengthof", "sizeof", "match", "omit":
				return
			}
		}
		if path := selectorPath(n); path != "" {
			for p, kind := range state {
				if kind == unionAltOK {
					continue
				}
				// Two flag conditions:
				//   - Strict deeper read of a bad
				//     parent (always wrong).
				//   - Same-path read when the bad
				//     kind is a value-shape mismatch
				//     (list / complement / null);
				//     for omit / `*` / ifpresent the
				//     same path itself is often the
				//     legitimate target of an
				//     introspection (already skipped
				//     above) or condition.
				if strings.HasPrefix(path, p+".") ||
					(path == p && isValueShapeBad(kind)) {
					diags = append(diags, Diagnostic{
						Code:     "union-alt-ref-bad-parent",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"referencing alternative %q is forbidden: parent path %q was assigned %s (ETSI 15.6.5)",
							path, p, kind.describe()),
						Node: n,
						Span: syntax.SpanOf(n),
					})
					return
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
	walk(expr)
	return diags
}

// classifyUnionAltRHS returns the bad-kind of an RHS expression. The
// shapes that block alternative references are listed at the top of
// this file. Anything else (single values, `?`, composite literals,
// function calls) is reported as `unionAltOK`.
func classifyUnionAltRHS(e syntax.Expr) unionAltBadKind {
	if e == nil {
		return unionAltOK
	}
	// `<expr> ifpresent` parses to UnaryExpr{Op: ifpresent}.
	if ue, ok := e.(*syntax.UnaryExpr); ok && ue != nil && ue.Op != nil &&
		ue.Op.Kind() == syntax.IFPRESENT {
		return unionAltIfPresent
	}
	switch v := e.(type) {
	case *syntax.Ident:
		if v == nil {
			return unionAltOK
		}
		switch v.String() {
		case "omit":
			return unionAltOmit
		case "null":
			return unionAltNull
		}
	case *syntax.ValueLiteral:
		if v == nil || v.Tok == nil {
			return unionAltOK
		}
		switch v.Tok.Kind() {
		case syntax.MUL:
			return unionAltAnyOrNone
		case syntax.OMIT:
			return unionAltOmit
		case syntax.NULL:
			return unionAltNull
		}
	case *syntax.ParenExpr:
		if v != nil && len(v.List) > 1 {
			return unionAltValueList
		}
	case *syntax.CallExpr:
		if v != nil {
			if id, ok := v.Fun.(*syntax.Ident); ok && id != nil && id.String() == "complement" {
				return unionAltComplement
			}
		}
	}
	return unionAltOK
}

// selectorPath flattens a SelectorExpr chain into a dotted path string
// like "a.b.c". Index expressions collapse to "[]" so a path like
// "a.b[5].c" is reported as "a.b.[].c" - good enough for prefix /
// suffix matching against the bad-path map. Returns "" if the
// expression is not a selector / index chain rooted at an Ident.
func selectorPath(e syntax.Node) string {
	switch v := e.(type) {
	case *syntax.Ident:
		if v == nil {
			return ""
		}
		return v.String()
	case *syntax.SelectorExpr:
		if v == nil || v.X == nil || v.Sel == nil {
			return ""
		}
		base := selectorPath(v.X)
		if base == "" {
			return ""
		}
		sel, ok := v.Sel.(*syntax.Ident)
		if !ok || sel == nil {
			return ""
		}
		return base + "." + sel.String()
	case *syntax.IndexExpr:
		if v == nil || v.X == nil {
			return ""
		}
		base := selectorPath(v.X)
		if base == "" {
			return ""
		}
		return base + ".[]"
	}
	return ""
}
