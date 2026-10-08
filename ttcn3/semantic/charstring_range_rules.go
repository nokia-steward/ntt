package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// charRangeBound captures one endpoint of a `("x" .. "y")` /
// `("x" .. !"y")` style constraint declared on a charstring or
// universal charstring subtype.
type charRangeBound struct {
	value     rune
	exclusive bool
}

type charRangeSpec struct {
	lo, hi    charRangeBound
	universal bool
	loSrc, hiSrc string
}

func (s charRangeSpec) contains(c rune) bool {
	if s.lo.exclusive {
		if c <= s.lo.value {
			return false
		}
	} else if c < s.lo.value {
		return false
	}
	if s.hi.exclusive {
		if c >= s.hi.value {
			return false
		}
	} else if c > s.hi.value {
		return false
	}
	return true
}

// checkCharRangeSubtypeRules flags assignments of literal strings
// whose characters fall outside the declared range of a
// (universal) charstring subtype (ETSI 6.1.1, 6.1.2.3).
func (a *Analyzer) checkCharRangeSubtypeRules(mod *syntax.Module) []Diagnostic {
	ranges := collectCharRangeSubtypes(mod)
	if len(ranges) == 0 {
		return nil
	}
	varTypes := collectModuleVarTypes(mod)
	if len(varTypes) == 0 {
		return nil
	}
	// Track each `var <baseT> x := <literal>` so that a later
	// `y := x;` (where y's type is a subtype of baseT) can be
	// validated against the subtype's range by looking the literal
	// up by name. Only single-init declarations are recorded; we
	// invalidate the entry on the first re-assignment we see.
	varLiteral := collectModuleVarInitLiterals(mod)
	var diags []Diagnostic
	check := func(typeName string, val syntax.Expr) {
		spec, ok := ranges[typeName]
		if !ok || val == nil {
			return
		}
		runes, raw, kind := extractCharsFromExpr(val, spec.universal)
		if kind == "" {
			// Indirect: `y := x;` where x is a value variable
			// initialised to a literal we already recorded.
			if id, ok := val.(*syntax.Ident); ok && id != nil {
				if lit, has := varLiteral[id.String()]; has && lit != nil {
					runes, raw, kind = extractCharsFromExpr(lit, spec.universal)
				}
			}
			if kind == "" {
				return
			}
		}
		for _, c := range runes {
			if !spec.contains(c) {
				display := raw
				if display == "" {
					display = fmt.Sprintf("%q", c)
				}
				diags = append(diags, Diagnostic{
					Code:     "charstring-range-out-of-range",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"value %s contains character %q outside %s range %s..%s (ETSI 6.1.2.3)",
						display, c, typeName, spec.loSrc, spec.hiSrc),
					Node: val,
					Span: syntax.SpanOf(val),
				})
				return
			}
		}
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			if v == nil {
				return true
			}
			ty := identName(v.Type)
			if ty == "" {
				return true
			}
			for _, d := range v.Decls {
				if d == nil || d.Value == nil {
					continue
				}
				check(ty, d.Value)
			}
		case *syntax.BinaryExpr:
			if v == nil || v.Op == nil || v.Op.Kind() != syntax.ASSIGN {
				return true
			}
			lhs, ok := v.X.(*syntax.Ident)
			if !ok || lhs == nil {
				return true
			}
			ty, has := varTypes[lhs.String()]
			if !has || ty == "" {
				return true
			}
			check(ty, v.Y)
		}
		return true
	})
	return diags
}

// collectModuleVarInitLiterals records each `var T x := <literal>`
// as an expr we can later look up by variable name. Only the
// FIRST initialisation wins; a second `var ... x` (shadow) or a
// subsequent `x := ...` invalidates the entry so we don't false-
// positive on values the program has already mutated.
func collectModuleVarInitLiterals(mod *syntax.Module) map[string]syntax.Expr {
	out := map[string]syntax.Expr{}
	seen := map[string]bool{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.ValueDecl:
			if v == nil || v.KindTok == nil ||
				v.KindTok.Kind() != syntax.VAR {
				return true
			}
			for _, d := range v.Decls {
				if d == nil || d.Name == nil {
					continue
				}
				name := d.Name.String()
				if seen[name] {
					delete(out, name)
					continue
				}
				seen[name] = true
				if d.Value == nil {
					continue
				}
				out[name] = d.Value
			}
		case *syntax.BinaryExpr:
			if v == nil || v.Op == nil || v.Op.Kind() != syntax.ASSIGN {
				return true
			}
			if id, ok := v.X.(*syntax.Ident); ok && id != nil {
				delete(out, id.String())
			}
		}
		return true
	})
	return out
}

// extractCharsFromExpr extracts the rune sequence from a value
// expression suitable for charstring range checking. It returns
// the rune slice, an optional source-text spelling for diagnostics,
// and a non-empty kind tag on success ("literal" or "char").
func extractCharsFromExpr(e syntax.Expr, universal bool) ([]rune, string, string) {
	switch x := e.(type) {
	case *syntax.ValueLiteral:
		if x == nil || x.Tok == nil {
			return nil, "", ""
		}
		raw := x.Tok.String()
		s := unquoteCharLit(raw)
		if s == "" && raw != `""` {
			return nil, "", ""
		}
		return []rune(s), raw, "literal"
	case *syntax.CallExpr:
		if !universal || x == nil || x.Fun == nil {
			return nil, "", ""
		}
		id, ok := x.Fun.(*syntax.Ident)
		if !ok || id == nil || id.String() != "char" {
			return nil, "", ""
		}
		if x.Args == nil || len(x.Args.List) != 4 {
			return nil, "", ""
		}
		var parts [4]int64
		for i, a := range x.Args.List {
			lit, ok := a.(*syntax.ValueLiteral)
			if !ok || lit == nil || lit.Tok == nil {
				return nil, "", ""
			}
			n, ok := parseIntLiteral(lit.Tok.String())
			if !ok || n < 0 || n > 0xFFFFFFFF {
				return nil, "", ""
			}
			parts[i] = n
		}
		cp := parts[0]<<24 | parts[1]<<16 | parts[2]<<8 | parts[3]
		return []rune{rune(cp)}, fmt.Sprintf("char(%d, %d, %d, %d)", parts[0], parts[1], parts[2], parts[3]), "char"
	}
	return nil, "", ""
}

// collectCharRangeSubtypes scans the module for `type charstring T
// (X..Y);` and `type universal charstring T (char(...)..char(...));`
// patterns and returns a map of type-name -> range spec.
func collectCharRangeSubtypes(mod *syntax.Module) map[string]charRangeSpec {
	out := map[string]charRangeSpec{}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		std, ok := n.(*syntax.SubTypeDecl)
		if !ok || std == nil || std.Field == nil || std.Field.Name == nil {
			return true
		}
		name := std.Field.Name.String()
		if name == "" {
			return true
		}
		base, isUniv := refSpecToCharBase(std.Field.Type)
		if base == "" {
			return true
		}
		if std.Field.ValueConstraint == nil || len(std.Field.ValueConstraint.List) == 0 {
			return true
		}
		bin, ok := std.Field.ValueConstraint.List[0].(*syntax.BinaryExpr)
		if !ok || bin == nil || bin.Op == nil || bin.Op.Kind() != syntax.RANGE {
			return true
		}
		lo, loSrc, okLo := charRangeEndpoint(bin.X, isUniv)
		hi, hiSrc, okHi := charRangeEndpoint(bin.Y, isUniv)
		if !okLo || !okHi {
			return true
		}
		spec := charRangeSpec{
			lo: lo, hi: hi,
			universal: isUniv,
			loSrc:     loSrc,
			hiSrc:     hiSrc,
		}
		if hi.exclusive && !strings.HasPrefix(spec.hiSrc, "!") {
			spec.hiSrc = "!" + spec.hiSrc
		}
		if lo.exclusive && !strings.HasPrefix(spec.loSrc, "!") {
			spec.loSrc = "!" + spec.loSrc
		}
		out[name] = spec
		return true
	})
	return out
}

func refSpecToCharBase(ts syntax.TypeSpec) (string, bool) {
	rs, ok := ts.(*syntax.RefSpec)
	if !ok || rs == nil {
		return "", false
	}
	name := identName(rs.X)
	switch name {
	case "charstring":
		return name, false
	case "universal charstring":
		return name, true
	}
	return "", false
}

// charRangeEndpoint extracts one endpoint of a (X..Y) charstring
// range. The endpoint may be prefixed with `!` (exclusive). For
// universal charstrings the endpoint is char(a,b,c,d).
func charRangeEndpoint(e syntax.Expr, universal bool) (charRangeBound, string, bool) {
	exclusive := false
	src := ""
	if u, ok := e.(*syntax.UnaryExpr); ok && u != nil && u.Op != nil && u.Op.Kind() == syntax.EXCL {
		exclusive = true
		e = u.X
		src = "!"
	}
	if universal {
		ce, ok := e.(*syntax.CallExpr)
		if !ok || ce == nil || ce.Fun == nil {
			return charRangeBound{}, "", false
		}
		id, ok := ce.Fun.(*syntax.Ident)
		if !ok || id == nil || id.String() != "char" {
			return charRangeBound{}, "", false
		}
		if ce.Args == nil || len(ce.Args.List) != 4 {
			return charRangeBound{}, "", false
		}
		var parts [4]int64
		for i, a := range ce.Args.List {
			lit, ok := a.(*syntax.ValueLiteral)
			if !ok || lit == nil || lit.Tok == nil {
				return charRangeBound{}, "", false
			}
			n, ok := parseIntLiteral(lit.Tok.String())
			if !ok || n < 0 || n > 0xFFFFFFFF {
				return charRangeBound{}, "", false
			}
			parts[i] = n
		}
		cp := parts[0]<<24 | parts[1]<<16 | parts[2]<<8 | parts[3]
		src += fmt.Sprintf("char(%d, %d, %d, %d)", parts[0], parts[1], parts[2], parts[3])
		return charRangeBound{value: rune(cp), exclusive: exclusive}, src, true
	}
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return charRangeBound{}, "", false
	}
	raw := lit.Tok.String()
	s := unquoteCharLit(raw)
	if len([]rune(s)) != 1 {
		return charRangeBound{}, "", false
	}
	src += raw
	return charRangeBound{value: []rune(s)[0], exclusive: exclusive}, src, true
}

func unquoteCharLit(raw string) string {
	if len(raw) < 2 {
		return ""
	}
	if raw[0] != '"' || raw[len(raw)-1] != '"' {
		return ""
	}
	inner := raw[1 : len(raw)-1]
	inner = strings.ReplaceAll(inner, `""`, `"`)
	return inner
}

func parseIntLiteral(s string) (int64, bool) {
	var n int64
	var i int
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	if i >= len(s) {
		return 0, false
	}
	for ; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, true
}
