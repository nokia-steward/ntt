// pattern_metachar_rules.go checks character patterns (ETSI ES 201 873-1
// annex B.1.5). A reference metacharacter {ref} shall not contain
// whitespace (B.1.5.0), and a template referred to as a pattern shall
// contain a specific value or a pattern, not a matching mechanism such as
// ? or * (B.1.5.0):
//
//	template charstring t1 := pattern "x{ c_ref }"; // error: whitespace
//	template charstring t2 := pattern mw_any;       // error: mw_any := ?
//
// \N{universal charstring} is not a set of a pattern of a charstring
// template (B.1.5.4, note 2). The quadruple \q{g, p, r, c}, the
// repetition #(n, m) and the type reference \N{universal charstring} are
// written with blanks in the standard itself and are let be.
package semantic

import (
	"strings"
	"unicode"

	"github.com/nokia/ntt/ttcn3"
	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPatternMetacharRules(tree *ttcn3.Tree, mod *syntax.Module) []Diagnostic {
	if tree == nil || mod == nil {
		return nil
	}
	m := typeModel{tree: tree, db: a.db()}
	var diags []Diagnostic
	report := func(n syntax.Node, code, msg string) {
		diags = append(diags, Diagnostic{Code: code, Severity: SeverityError, Message: msg, Node: n, Span: syntax.SpanOf(n)})
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		pe, ok := n.(*syntax.PatternExpr)
		if !ok || pe.X == nil {
			return true
		}
		switch x := unparen(pe.X).(type) {
		case *syntax.ValueLiteral:
			if x.Tok != nil && x.Tok.Kind() == syntax.STRING {
				if meta := spacedMetachar(x.Tok.String()); meta != "" {
					report(x, "pattern-metachar-whitespace", "the metacharacter "+meta+" of the pattern contains whitespace (ETSI B.1.5.0)")
				}
				if strings.Contains(x.Tok.String(), `\N{universal charstring}`) && m.patternOfCharstring(pe) {
					report(x, "pattern-universal-set-in-charstring", `\N{universal charstring} matches characters a charstring template does not have (ETSI B.1.5.4)`)
				}
			}
		case *syntax.Ident:
			if body := m.templateBody(x); body != nil && !patternSource(body) {
				report(x, "pattern-reference-not-specific", x.String()+" is referred to as a pattern but contains neither a specific value nor a pattern (ETSI B.1.5.0)")
			}
		}
		return true
	})
	return diags
}

// spacedMetachar returns the first reference metacharacter {ref} of the
// pattern literal lit that contains whitespace, or "".
func spacedMetachar(lit string) string {
	s := strings.TrimSuffix(strings.TrimPrefix(lit, `"`), `"`)
	inSet := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\':
			// An escaped character, or \q{...} and \N{...} whole.
			i++
			if i+1 < len(s) && (s[i] == 'q' || s[i] == 'N') && s[i+1] == '{' {
				if j := strings.IndexByte(s[i:], '}'); j >= 0 {
					i += j
				}
			}
		case c == '[':
			inSet = true
		case c == ']':
			inSet = false
		case c == '{' && !inSet:
			j := strings.IndexByte(s[i+1:], '}')
			if j < 0 {
				return ""
			}
			if strings.IndexFunc(s[i+1:i+1+j], unicode.IsSpace) >= 0 {
				return s[i : i+2+j]
			}
			i += 1 + j
		}
	}
	return ""
}

// patternOfCharstring reports whether pattern pe is the body of a
// template or template variable of a charstring type.
func (m typeModel) patternOfCharstring(pe *syntax.PatternExpr) bool {
	var typ syntax.Expr
	switch p := m.tree.ParentOf(pe).(type) {
	case *syntax.TemplateDecl:
		typ = p.Type
	case *syntax.Declarator:
		if vd, ok := m.tree.ParentOf(p).(*syntax.ValueDecl); ok {
			typ = vd.Type
		}
	}
	if typ == nil {
		return false
	}
	r, ok := m.typeRoot(m.tree, typ, 0)
	return ok && r.kind == "charstring"
}

// patternSource reports whether a template body can be referred to as a
// pattern: anything but a matching mechanism that is not a pattern.
func patternSource(e syntax.Expr) bool {
	switch x := unparen(e).(type) {
	case *syntax.ValueLiteral:
		if x.Tok == nil {
			return true
		}
		switch x.Tok.Kind() {
		case syntax.ANY, syntax.MUL, syntax.OMIT:
			return false
		}
	case *syntax.ParenExpr:
		// A value list.
		return false
	case *syntax.CallExpr:
		switch syntax.Name(x.Fun) {
		case "complement", "subset", "superset", "permutation":
			return false
		}
	}
	return true
}
