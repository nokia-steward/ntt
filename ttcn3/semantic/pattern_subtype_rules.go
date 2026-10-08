// pattern_subtype_rules.go enforces a tiny subset of
// ETSI ES 201 873-1 clause 6.1.2.5: when a charstring /
// universal charstring sub-type carries a `pattern`
// restriction, any literal value bound to a variable of
// that sub-type must match the pattern.
//
// We only handle the cleanest shape:
//   - the pattern source is a quoted ASCII literal that
//     contains only `?`, `*`, space, `-`, `_` and the
//     standard alphanumerics,
//   - the value literal is a quoted ASCII string without
//     embedded escapes or quadruple notation,
//   - the variable type is the sub-type name itself
//     (no nesting through further sub-types).
//
// Patterns that use any other TTCN-3 metacharacter
// (`[...]`, `\d`, `\q{}`, alternation, repetition, etc.)
// are skipped because the simplified regex translation
// would not faithfully model them.
package semantic

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPatternSubtypeRules(mod *syntax.Module) []Diagnostic {
	if mod == nil {
		return nil
	}
	patterns := collectPatternSubtypes(mod)
	if len(patterns) == 0 {
		return nil
	}
	var diags []Diagnostic
	varToPattern := map[string]patternInfo{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		typeID, ok := vd.Type.(*syntax.Ident)
		if !ok || typeID == nil {
			return true
		}
		pat, ok := patterns[typeID.String()]
		if !ok {
			return true
		}
		for _, dc := range vd.Decls {
			if dc == nil || dc.Name == nil {
				continue
			}
			varToPattern[dc.Name.String()] = pat
			if dc.Value == nil {
				continue
			}
			checkPatternLiteralAgainst(dc.Value, pat, typeID.String(), dc, &diags)
		}
		return true
	})
	syntax.Inspect(mod, func(n syntax.Node) bool {
		be, ok := n.(*syntax.BinaryExpr)
		if !ok || be == nil || be.Op == nil || be.Op.Kind() != syntax.ASSIGN {
			return true
		}
		id, ok := be.X.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		pat, ok := varToPattern[id.String()]
		if !ok {
			return true
		}
		checkPatternLiteralAgainst(be.Y, pat, id.String(), be, &diags)
		return true
	})
	return diags
}

func checkPatternLiteralAgainst(
	expr syntax.Expr,
	pat patternInfo,
	typeOrVarName string,
	node syntax.Node,
	diags *[]Diagnostic,
) {
	vl, ok := expr.(*syntax.ValueLiteral)
	if !ok || vl == nil || vl.Tok == nil {
		return
	}
	value, ok := simpleAsciiString(vl.Tok.String())
	if !ok {
		return
	}
	if pat.regex.MatchString(value) {
		return
	}
	*diags = append(*diags, Diagnostic{
		Code:     "pattern-subtype-mismatch",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"value %q does not match the `pattern` constraint %q of sub-type %q (ETSI 6.1.2.5)",
			value, pat.raw, typeOrVarName),
		Node: node,
		Span: syntax.SpanOf(node),
	})
}

type patternInfo struct {
	raw   string
	regex *regexp.Regexp
}

func collectPatternSubtypes(mod *syntax.Module) map[string]patternInfo {
	out := map[string]patternInfo{}
	for _, d := range mod.Defs {
		if d == nil {
			continue
		}
		std, ok := d.Def.(*syntax.SubTypeDecl)
		if !ok || std == nil || std.Field == nil ||
			std.Field.Name == nil ||
			std.Field.ValueConstraint == nil {
			continue
		}
		var pat *syntax.PatternExpr
		for _, c := range std.Field.ValueConstraint.List {
			if pe, ok := c.(*syntax.PatternExpr); ok && pe != nil {
				pat = pe
				break
			}
		}
		if pat == nil {
			continue
		}
		if pat.NoCase != nil {
			continue
		}
		lit, ok := pat.X.(*syntax.ValueLiteral)
		if !ok || lit == nil || lit.Tok == nil {
			continue
		}
		raw, ok := simpleAsciiString(lit.Tok.String())
		if !ok {
			continue
		}
		re, ok := simplePatternToRegex(raw)
		if !ok {
			continue
		}
		out[std.Field.Name.String()] = patternInfo{raw: raw, regex: re}
	}
	return out
}

func simpleAsciiString(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	body := raw[1 : len(raw)-1]
	if strings.ContainsAny(body, "\\") {
		return "", false
	}
	for _, r := range body {
		if r > 0x7e || r < 0x20 {
			return "", false
		}
	}
	return body, true
}

func simplePatternToRegex(p string) (*regexp.Regexp, bool) {
	var b strings.Builder
	b.WriteByte('^')
	for _, r := range p {
		switch r {
		case '?':
			b.WriteByte('.')
		case '*':
			b.WriteString(".*")
		case '[', ']', '{', '}', '(', ')', '|', '+', '\\', '.', '^', '$':
			return nil, false
		default:
			if r >= '0' && r <= '9' ||
				r >= 'a' && r <= 'z' ||
				r >= 'A' && r <= 'Z' ||
				r == ' ' || r == '_' || r == '-' {
				b.WriteRune(r)
				continue
			}
			return nil, false
		}
	}
	b.WriteByte('$')
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, false
	}
	return re, true
}
