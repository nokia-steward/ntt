// select_stmt_rules.go implements the ETSI ES 201 873-1 clause
// 19.3 rules for `select case` and `select union`:
//
//   - 19.3.1 b: when every case branch's template instance can
//     be statically evaluated to a specific value, no two
//     branches shall match the same value (duplicate case
//     literals are rejected).
//   - 19.3.2 a: the header of a `select union` statement must
//     be of a union type. We treat any declared non-union
//     type (record, set, record-of, enum, port, ...) as a
//     reject.
//   - 19.3.2 b: no two cases in a `select union` shall share the
//     same case identifier.
//   - 19.3.2 c: each case identifier of a `select union` must
//     be one of the union's alternative names. Unknown
//     identifiers are flagged.
//
// All rules are intentionally local: we only consult the
// module-level type declarations and the per-function var/param
// scope; cross-module type resolution is left to a later pass.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkSelectStmtRules(mod *syntax.Module) []Diagnostic {
	unions := collectUnionMembers(mod)
	declKinds := collectDeclaredTypeKinds(mod)

	var diags []Diagnostic
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		fn, ok := d.Def.(*syntax.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		varTypes := collectVarDeclaredTypeNames(fn.Body)
		paramTypes := collectFormalParamTypeNames(fn.Params)
		syntax.Inspect(fn.Body, func(n syntax.Node) bool {
			ss, ok := n.(*syntax.SelectStmt)
			if !ok || ss == nil {
				return true
			}
			if ss.Union != nil {
				diags = append(diags, checkSelectUnion(ss, unions, declKinds, varTypes, paramTypes)...)
			} else {
				diags = append(diags, checkSelectCase(ss)...)
			}
			return true
		})
	}
	return diags
}

// collectUnionMembers maps every union type declared in the
// module to the ordered list of its alternative identifier
// names. Used by checkSelectUnion to validate case identifiers.
func collectUnionMembers(mod *syntax.Module) map[string][]string {
	out := map[string][]string{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.StructTypeDecl)
		if !ok || st.Name == nil {
			continue
		}
		// We only care about union types here.
		if st.KindTok == nil || st.KindTok.Kind() != syntax.UNION {
			continue
		}
		var names []string
		for _, f := range st.Fields {
			if f == nil || f.Name == nil {
				continue
			}
			names = append(names, f.Name.String())
		}
		out[st.Name.String()] = names
	}
	return out
}

// checkSelectCase enforces the duplicate-case-literal rule for
// `select case` statements. Each case may list one or more
// literals: we walk every literal and report when the same
// literal appears twice across the entire select.
func checkSelectCase(ss *syntax.SelectStmt) []Diagnostic {
	seen := map[string]int{}
	var diags []Diagnostic
	for i, cc := range ss.Body {
		if cc == nil || cc.Case == nil {
			continue
		}
		for _, expr := range cc.Case.List {
			key := caseLiteralKey(expr)
			if key == "" {
				continue
			}
			if prev, ok := seen[key]; ok {
				diags = append(diags, Diagnostic{
					Code:     "select-case-duplicate-literal",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"select case branch %d duplicates a literal already used by branch %d (%s)",
						i+1, prev+1, key),
					Node: expr,
					Span: syntax.SpanOf(expr),
				})
				continue
			}
			seen[key] = i
		}
	}
	return diags
}

// caseLiteralKey returns a string key uniquely identifying the
// literal value of a select-case branch, or "" when the
// expression is not a recognisable literal we can compare
// statically.
func caseLiteralKey(e syntax.Expr) string {
	switch v := e.(type) {
	case *syntax.ValueLiteral:
		if v.Tok != nil {
			return v.Tok.Kind().String() + ":" + v.Tok.String()
		}
	case *syntax.UnaryExpr:
		// `-5` form: signed integer literal.
		if v.Op != nil && v.X != nil {
			inner := caseLiteralKey(v.X)
			if inner != "" {
				return v.Op.String() + inner
			}
		}
	}
	return ""
}

// checkSelectUnion enforces the header-must-be-union, no
// duplicate-case-identifier, and case-identifier-must-be-a-union
// -alternative rules.
func checkSelectUnion(ss *syntax.SelectStmt, unions map[string][]string, declKinds map[string]typeKind, varTypes, paramTypes map[string]string) []Diagnostic {
	var diags []Diagnostic

	// Header type resolution: the tag is a parenthesised
	// expression containing a single value. We pull the inner
	// ident, look it up in var / param scope, and check the
	// declared type is in `unions`.
	headerName := ""
	headerType := ""
	if ss.Tag != nil && len(ss.Tag.List) == 1 {
		if id, ok := ss.Tag.List[0].(*syntax.Ident); ok && id != nil && id.Tok != nil {
			headerName = id.String()
			if t, ok := varTypes[headerName]; ok {
				headerType = t
			} else if t, ok := paramTypes[headerName]; ok {
				headerType = t
			}
		}
	}
	memberSet := map[string]bool{}
	if headerType != "" {
		if _, ok := unions[headerType]; !ok {
			// Header is a known type but not a union. Reject.
			if kind, known := declKinds[headerType]; known && kind != tkComponent {
				diags = append(diags, Diagnostic{
					Code:     "select-union-non-union-header",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"select union (%s): header type %q is not a union type (ETSI 19.3.2)",
						headerName, headerType),
					Node: ss,
					Span: syntax.SpanOf(ss),
				})
				return diags
			}
		} else {
			for _, m := range unions[headerType] {
				memberSet[m] = true
			}
		}
	}

	seen := map[string]int{}
	for i, cc := range ss.Body {
		if cc == nil || cc.Case == nil {
			continue
		}
		for _, expr := range cc.Case.List {
			id, ok := expr.(*syntax.Ident)
			if !ok || id == nil || id.Tok == nil {
				continue
			}
			name := id.String()
			if prev, ok := seen[name]; ok {
				diags = append(diags, Diagnostic{
					Code:     "select-union-duplicate-case",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"select union case %q appears in branch %d and %d",
						name, prev+1, i+1),
					Node: cc,
					Span: syntax.SpanOf(cc),
				})
				continue
			}
			seen[name] = i
			if len(memberSet) > 0 && !memberSet[name] {
				diags = append(diags, Diagnostic{
					Code:     "select-union-unknown-case",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"select union case %q is not an alternative of union %q",
						name, headerType),
					Node: cc,
					Span: syntax.SpanOf(cc),
				})
			}
		}
	}
	return diags
}
