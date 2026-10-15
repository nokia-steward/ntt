// predefined_fn_rules.go enforces a small subset of the
// predefined-function arity / type rules from ETSI ES 201 873-1
// Annex C:
//
//   - sizeof(x) requires x to be a fixed-size structured type or
//     template thereof. Record-of, set-of, union, anytype and
//     enumerated templates are rejected (lengthof should be used
//     for variable-size lists).
//
//   - regexp(s, pattern, groupIndex) requires groupIndex >= 0.
//     The pattern itself is generally not introspectable
//     statically so we only reject the trivially out-of-range
//     literal `-1` (and similar negative integer literals).
//
// These are narrow rules and only fire on call shapes the
// parser surfaces directly. Anything wrapped in an expression
// falls through.
package semantic

import (
	"fmt"
	"strconv"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPredefinedFunctionRules(mod *syntax.Module) []Diagnostic {
	typeCats := collectTypeCategories(mod)
	tmplTypes := collectTemplateTypeNames(mod)
	varTypes := collectVarTypeNames(mod)

	var diags []Diagnostic
	syntax.Inspect(mod, func(n syntax.Node) bool {
		ce, ok := n.(*syntax.CallExpr)
		if !ok || ce == nil {
			return true
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		switch id.String() {
		case "sizeof":
			diags = append(diags, checkSizeofCall(ce, typeCats, tmplTypes, varTypes)...)
		case "rnd":
			diags = append(diags, checkRndCall(ce)...)
		case "substr":
			diags = append(diags, checkSubstrCall(ce, mod, tmplTypes)...)
		}
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		re, ok := n.(*syntax.RegexpExpr)
		if !ok || re == nil {
			return true
		}
		diags = append(diags, checkRegexpExpr(re)...)
		return true
	})
	return diags
}

func checkSizeofCall(
	ce *syntax.CallExpr,
	typeCats map[string]typeCategory,
	tmplTypes map[string]string,
	varTypes map[string]string,
) []Diagnostic {
	if ce.Args == nil || len(ce.Args.List) != 1 {
		return nil
	}
	arg := ce.Args.List[0]
	name := identName(arg)
	if name == "" {
		return nil
	}
	typeName := tmplTypes[name]
	if typeName == "" {
		typeName = varTypes[name]
	}
	if typeName == "" {
		return nil
	}
	var reason string
	if typeName == "anytype" {
		reason = "anytype (sizeof is only defined for record / set / array)"
	} else {
		cat, ok := typeCats[typeName]
		if !ok {
			return nil
		}
		switch cat {
		case typeCategoryRecordOf:
			reason = "record of (use lengthof for variable-length lists)"
		case typeCategorySetOf:
			reason = "set of (use lengthof for variable-length lists)"
		case typeCategoryUnion:
			reason = "union (sizeof is only defined for fixed-size structures)"
		}
	}
	if reason == "" {
		return nil
	}
	return []Diagnostic{{
		Code:     "sizeof-on-variable-shape",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"sizeof(%s): argument has type %q which is %s",
			name, typeName, reason),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}}
}

func checkRegexpExpr(re *syntax.RegexpExpr) []Diagnostic {
	if re == nil || re.X == nil {
		return nil
	}
	pe, ok := re.X.(*syntax.ParenExpr)
	if !ok || pe == nil {
		return nil
	}
	if len(pe.List) < 3 {
		return []Diagnostic{{
			Code:     "regexp-missing-group-index",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"regexp(...): missing group index (regexp takes 3 arguments: input, pattern, groupNo; got %d)",
				len(pe.List)),
			Node: re,
			Span: syntax.SpanOf(re),
		}}
	}
	var diags []Diagnostic
	groupArg := pe.List[2]
	v, vOk := signedIntLiteral(groupArg)
	if vOk && v < 0 {
		diags = append(diags, Diagnostic{
			Code:     "regexp-negative-group-index",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"regexp(...): group index %d is negative (must be >= 0)",
				v),
			Node: re,
			Span: syntax.SpanOf(re),
		})
	}
	if vOk && v >= 0 {
		if count, ok := regexpGroupCount(pe.List[1]); ok && int64(count) <= v {
			diags = append(diags, Diagnostic{
				Code:     "regexp-group-index-out-of-range",
				Severity: SeverityError,
				Message: fmt.Sprintf(
					"regexp(...): group index %d is out of range (pattern has %d capturing group%s)",
					v, count, plural(count)),
				Node: re,
				Span: syntax.SpanOf(re),
			})
		}
	}
	return diags
}

// regexpGroupCount returns the number of capturing groups
// (top-level `(...)` pairs) inside a literal string pattern. We
// only inspect literal patterns of the shape `Type:"..."` or
// bare `"..."`. References to a named template / constant fall
// through silently (ok=false).
//
// The counter strips `\(` escape sequences. Nested groups still
// count as separate captures.
func regexpGroupCount(arg syntax.Expr) (int, bool) {
	lit := stringLiteralOf(arg)
	if lit == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(lit); i++ {
		if lit[i] == '\\' {
			i++
			continue
		}
		if lit[i] == '(' {
			n++
		}
	}
	return n, true
}

// stringLiteralOf extracts the raw content of a `"..."` literal
// expression. Three shapes are recognised:
//
//	"..."                    bare ValueLiteral{STRING}
//	Type:"..."               BinaryExpr COLON (typed form)
//	("...")                  ParenExpr wrapping the above
//
// Returns the unquoted text or "" when the argument isn't a
// string literal we can statically read.
func stringLiteralOf(e syntax.Expr) string {
	if e == nil {
		return ""
	}
	if pe, ok := e.(*syntax.ParenExpr); ok && len(pe.List) == 1 {
		return stringLiteralOf(pe.List[0])
	}
	if be, ok := e.(*syntax.BinaryExpr); ok && be.Op != nil && be.Op.Kind() == syntax.COLON {
		return stringLiteralOf(be.Y)
	}
	if lit, ok := e.(*syntax.ValueLiteral); ok && lit != nil && lit.Tok != nil {
		if lit.Tok.Kind() != syntax.STRING {
			return ""
		}
		s := lit.Tok.String()
		if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
		return s
	}
	return ""
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// signedIntLiteral parses a literal integer expression. The
// parser surfaces `-1` as a UnaryExpr with a SUB operator and
// an INT child; bare positive ints are ValueLiteral{INT}.
func signedIntLiteral(e syntax.Expr) (int64, bool) {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x == nil || x.Tok == nil || x.Tok.Kind() != syntax.INT {
			return 0, false
		}
		v, err := strconv.ParseInt(x.Tok.String(), 10, 64)
		if err != nil {
			return 0, false
		}
		return v, true
	case *syntax.UnaryExpr:
		if x == nil || x.Op == nil {
			return 0, false
		}
		child, ok := signedIntLiteral(x.X)
		if !ok {
			return 0, false
		}
		switch x.Op.Kind() {
		case syntax.SUB:
			return -child, true
		case syntax.ADD:
			return child, true
		}
	}
	return 0, false
}

func collectTemplateTypeNames(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		out[td.Name.String()] = syntax.Name(td.Type)
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		isTemplate := false
		if vd.KindTok != nil && vd.KindTok.Kind() == syntax.TEMPLATE {
			isTemplate = true
		}
		if vd.TemplateRestriction != nil {
			isTemplate = true
		}
		if !isTemplate {
			return true
		}
		typeName := syntax.Name(vd.Type)
		if typeName == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = typeName
		}
		return true
	})
	return out
}

// checkRndCall rejects the literal-seed forms `rnd(infinity)`,
// `rnd(-infinity)` and `rnd(not_a_number)`. ETSI ES 201 873-1
// (Annex C.5.6.2) requires a finite floating-point seed.
func checkRndCall(ce *syntax.CallExpr) []Diagnostic {
	if ce == nil || ce.Args == nil || len(ce.Args.List) != 1 {
		return nil
	}
	arg := ce.Args.List[0]
	name, ok := nonFiniteFloatName(arg)
	if !ok {
		return nil
	}
	return []Diagnostic{{
		Code:     "rnd-non-finite-seed",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"rnd(...): seed %s is not a finite floating-point value",
			name),
		Node: ce,
		Span: syntax.SpanOf(ce),
	}}
}

func nonFiniteFloatName(e syntax.Expr) (string, bool) {
	switch x := e.(type) {
	case *syntax.Ident:
		if x == nil {
			return "", false
		}
		switch x.String() {
		case "infinity", "not_a_number":
			return x.String(), true
		}
	case *syntax.UnaryExpr:
		if x == nil || x.Op == nil {
			return "", false
		}
		if x.Op.Kind() != syntax.SUB {
			return "", false
		}
		if inner, ok := nonFiniteFloatName(x.X); ok {
			return "-" + inner, true
		}
	}
	return "", false
}

// checkSubstrCall rejects substr(inpar, ...) when the inpar
// template is statically known to contain a non-AnyElement
// matching mechanism (e.g. `*` in a bit/hex/octet literal or
// `*` inside a record-of composite literal).
//
// ETSI ES 201 873-1 Annex C.3.1 lists this restriction as one
// of the extra error conditions specific to substr beyond the
// general clause 16.1.2 rules.
func checkSubstrCall(
	ce *syntax.CallExpr,
	mod *syntax.Module,
	tmplTypes map[string]string,
) []Diagnostic {
	if ce == nil || ce.Args == nil || len(ce.Args.List) < 1 {
		return nil
	}
	arg := ce.Args.List[0]
	name := identName(arg)
	if name == "" {
		return nil
	}
	if _, isTmpl := tmplTypes[name]; !isTmpl {
		return nil
	}
	init := templateInitOf(mod, name)
	if init == nil {
		return nil
	}
	if reason, ok := containsForbiddenSubstrMatch(init); ok {
		return []Diagnostic{{
			Code:     "substr-forbidden-matching-mechanism",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"substr(%s, ...): template contains %s, which is forbidden "+
					"(only AnyElement is allowed inside substr's inpar)",
				name, reason),
			Node: ce,
			Span: syntax.SpanOf(ce),
		}}
	}
	return nil
}

func templateInitOf(mod *syntax.Module, name string) syntax.Expr {
	var found syntax.Expr
	syntax.Inspect(mod, func(n syntax.Node) bool {
		td, ok := n.(*syntax.TemplateDecl)
		if !ok || td == nil || td.Name == nil {
			return true
		}
		if td.Name.String() != name {
			return true
		}
		if td.Value != nil {
			found = td.Value
		}
		return false
	})
	if found != nil {
		return found
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			if dec.Name.String() == name && dec.Value != nil {
				found = dec.Value
				return false
			}
		}
		return true
	})
	return found
}

// containsForbiddenSubstrMatch detects matching mechanisms in
// the static initializer of a substr inpar template. Currently
// recognised:
//   - `*` inside a bit/hex/octet/character string literal
//   - bare `*` inside a composite literal (record-of / array)
func containsForbiddenSubstrMatch(e syntax.Expr) (string, bool) {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x == nil || x.Tok == nil {
			return "", false
		}
		s := x.Tok.String()
		if !looksLikeQuotedBinaryLiteral(s) && !looksLikeQuotedCharLiteral(s) {
			return "", false
		}
		body := stripLiteralQuotes(s)
		if containsUnescapedStar(body) {
			return "AnyElementsOrNone (`*`) inside a string literal", true
		}
	case *syntax.CompositeLiteral:
		if x == nil {
			return "", false
		}
		for _, el := range x.List {
			if isBareAnyElementsOrNone(el) {
				return "AnyElementsOrNone (`*`) as a list element", true
			}
		}
	}
	return "", false
}

func looksLikeQuotedBinaryLiteral(s string) bool {
	if len(s) < 3 {
		return false
	}
	last := s[len(s)-1]
	if last != 'B' && last != 'H' && last != 'O' {
		return false
	}
	return s[0] == '\''
}

func looksLikeQuotedCharLiteral(s string) bool {
	if len(s) < 2 {
		return false
	}
	return s[0] == '"' && s[len(s)-1] == '"'
}

func stripLiteralQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	if len(s) >= 3 && s[0] == '\'' {
		end := len(s) - 1
		for end > 0 && s[end] != '\'' {
			end--
		}
		if end > 0 {
			return s[1:end]
		}
	}
	return s
}

func containsUnescapedStar(body string) bool {
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' {
			i++
			continue
		}
		if body[i] == '*' {
			return true
		}
	}
	return false
}

func isBareAnyElementsOrNone(e syntax.Expr) bool {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return false
	}
	return lit.Tok.Kind() == syntax.MUL
}

func collectVarTypeNames(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeName := syntax.Name(vd.Type)
		if typeName == "" {
			return true
		}
		for _, dec := range vd.Decls {
			if dec == nil || dec.Name == nil {
				continue
			}
			out[dec.Name.String()] = typeName
		}
		return true
	})
	return out
}
