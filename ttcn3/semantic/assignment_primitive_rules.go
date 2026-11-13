// assignment_primitive_rules.go enforces a narrow slice of ETSI
// ES 201 873-1 clause 19.1 / 6.3.1 "non-structured type
// compatibility": certain literal-shape assignment violations can
// be caught with no type resolution beyond the declared name of
// the left-hand side variable.
//
// Rules implemented here:
//
//   - assign-float-to-integer: a float literal (`1.5`, `1.0e2`)
//     cannot be assigned (or used as initializer) for a variable
//     declared as `integer` - the two basic types are not
//     compatible (ETSI 6.3.1 / 19.1).
//   - positional-omit-on-list-element: each `omit` element of a
//     positional CompositeLiteral assigned to a `record of T`,
//     `set of T`, or fixed-size array variable is forbidden,
//     because list / array elements are mandatory (only `record`
//     and `set` named fields can be marked `optional`).
//
// The collateral damage surface is intentionally small: the
// integer-float check only triggers on literal RHS expressions
// (no operator chains, no function calls); the list-of-omit
// check only triggers on positional literals (no field := value
// form which is already covered by `checkOmitValueRules`).
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkAssignmentPrimitiveRules(mod *syntax.Module) []Diagnostic {
	intVars := collectIntegerVarNames(mod)
	listVarKinds := collectListLikeVarKinds(mod)
	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			if v == nil || v.KindTok == nil || v.KindTok.Kind() != syntax.VAR {
				return true
			}
			// `var template T x` may carry template-only
			// constructs in its initializer and is out of
			// scope for both checks.
			if v.TemplateRestriction != nil &&
				v.TemplateRestriction.TemplateTok != nil &&
				v.TemplateRestriction.TemplateTok.Kind() != syntax.ILLEGAL {
				return true
			}
			declTypeName := identName(v.Type)
			declIsInt := declTypeName == "integer"
			for _, d := range v.Decls {
				if d == nil || d.Value == nil {
					continue
				}
				declHasArray := len(d.ArrayDef) > 0
				if declIsInt && !declHasArray {
					if isFloatLiteralLike(d.Value) {
						diags = append(diags, floatToIntDiag(d.Value, declName(d)))
					}
				}
				// `var integer x[2] := {11, omit}` --
				// the LHS is an array, omit is forbidden.
				if cl, ok := d.Value.(*syntax.CompositeLiteral); ok {
					if declHasArray {
						diags = append(diags, positionalOmitDiags(
							cl, declName(d), "array")...)
					} else if kind, ok := listVarKinds[declName(d)]; ok {
						diags = append(diags, positionalOmitDiags(
							cl, declName(d), kind)...)
					}
				}
			}
		case *syntax.BinaryExpr:
			if v == nil || v.Op == nil || v.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhsId, ok := v.X.(*syntax.Ident)
			if !ok || lhsId == nil {
				return true
			}
			name := lhsId.String()
			if intVars[name] && isFloatLiteralLike(v.Y) {
				diags = append(diags, floatToIntDiag(v.Y, name))
			}
			if cl, ok := v.Y.(*syntax.CompositeLiteral); ok {
				if kind, ok := listVarKinds[name]; ok {
					diags = append(diags, positionalOmitDiags(
						cl, name, kind)...)
				}
			}
		}
		return true
	})
	return diags
}

// collectIntegerVarNames builds the set of `var integer name`
// declarations in the module. Arrays are excluded (`var integer x[2]`
// is a list of integers, where `x := 1.5` is a different category).
func collectIntegerVarNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil &&
			vd.TemplateRestriction.TemplateTok.Kind() != syntax.ILLEGAL {
			return true
		}
		if identName(vd.Type) != "integer" {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil || len(d.ArrayDef) > 0 {
				continue
			}
			out[d.Name.String()] = true
		}
		return true
	})
	return out
}

// collectListLikeVarKinds maps every variable declared with a list-
// or array-shaped type (set of / record of / array `var T x[N]`)
// to a short kind string used in diagnostics. The kind is computed
// from the *declared type*: either by following a named subtype
// declaration (`type set of integer Myset; var Myset v;`) or from
// the declarator's own ArrayDef (`var integer v[2];`).
func collectListLikeVarKinds(mod *syntax.Module) map[string]string {
	listSubtypes := collectListSubtypeKinds(mod)
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil || vd.KindTok == nil ||
			vd.KindTok.Kind() != syntax.VAR {
			return true
		}
		if vd.TemplateRestriction != nil &&
			vd.TemplateRestriction.TemplateTok != nil &&
			vd.TemplateRestriction.TemplateTok.Kind() != syntax.ILLEGAL {
			return true
		}
		declType := identName(vd.Type)
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			if len(d.ArrayDef) > 0 {
				out[d.Name.String()] = "array"
				continue
			}
			if kind, ok := listSubtypes[declType]; ok {
				out[d.Name.String()] = kind
			}
		}
		return true
	})
	return out
}

// collectListSubtypeKinds returns name -> "record of"/"set of" for
// every `type X NAME ;` declaration where X is a ListSpec. Named
// subtypes whose base is itself a previously-declared list type
// are also recorded so `type set of integer Myset; type Myset Mw;`
// surfaces both.
func collectListSubtypeKinds(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	for _, d := range mod.Defs {
		if d == nil || d.Def == nil {
			continue
		}
		st, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
			continue
		}
		if ls, ok := st.Field.Type.(*syntax.ListSpec); ok && ls != nil &&
			ls.KindTok != nil {
			kind := ls.KindTok.String()
			if ls.OfTok != nil {
				kind = kind + " of"
			}
			out[st.Field.Name.String()] = kind
		}
	}
	// Fixpoint: resolve chained typedefs `type Myset Other`.
	for changed := true; changed; {
		changed = false
		for _, d := range mod.Defs {
			if d == nil || d.Def == nil {
				continue
			}
			st, ok := d.Def.(*syntax.SubTypeDecl)
			if !ok || st == nil || st.Field == nil || st.Field.Name == nil {
				continue
			}
			name := st.Field.Name.String()
			if _, done := out[name]; done {
				continue
			}
			baseName := ""
			if rs, ok := st.Field.Type.(*syntax.RefSpec); ok && rs != nil {
				baseName = identName(rs.X)
			}
			if baseName == "" {
				continue
			}
			if kind, ok := out[baseName]; ok {
				out[name] = kind
				changed = true
			}
		}
	}
	return out
}

// isFloatLiteralLike reports whether e is a float literal,
// optionally with a leading unary +/-. Nested expressions and
// function calls are not treated as literals.
func isFloatLiteralLike(e syntax.Expr) bool {
	switch x := e.(type) {
	case *syntax.UnaryExpr:
		if x == nil || x.Op == nil || x.X == nil {
			return false
		}
		switch x.Op.Kind() {
		case syntax.ADD, syntax.SUB:
			return isFloatLiteralLike(x.X)
		}
		return false
	case *syntax.ValueLiteral:
		if x == nil || x.Tok == nil {
			return false
		}
		return x.Tok.Kind() == syntax.FLOAT
	}
	return false
}

// floatToIntDiag formats the assign-float-to-integer diagnostic.
func floatToIntDiag(rhs syntax.Expr, name string) Diagnostic {
	return Diagnostic{
		Code:     "assign-float-to-integer",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"integer variable %q cannot be assigned a float literal (ETSI 6.3.1 / 19.1)",
			name),
		Node: rhs,
		Span: syntax.SpanOf(rhs),
	}
}

// positionalOmitDiags scans a positional composite literal for
// `omit` elements and emits one diagnostic per occurrence. Named
// element entries (`field := X`) are handed off to the
// record-field optionality rule and skipped here.
func positionalOmitDiags(cl *syntax.CompositeLiteral, name, kind string) []Diagnostic {
	if cl == nil {
		return nil
	}
	var diags []Diagnostic
	for _, item := range cl.List {
		if be, ok := item.(*syntax.BinaryExpr); ok && be != nil && be.Op != nil &&
			be.Op.Kind() == syntax.ASSIGN {
			continue
		}
		if !isOmitLiteral(item) {
			continue
		}
		diags = append(diags, Diagnostic{
			Code:     "positional-omit-on-list-element",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"variable %q is a %s; elements are mandatory and cannot be omit (ETSI 19.1 c)",
				name, kind),
			Node: item,
			Span: syntax.SpanOf(item),
		})
	}
	return diags
}
