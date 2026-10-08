// Package attr parses TTCN-3 `with { ... }` attribute clauses (TTCN-3 v4.11.1
// Annex E) into a typed structure that codecs and the runtime can consume
// directly, instead of re-parsing free-form strings at every call site.
//
// The TTCN-3 surface syntax is small but ambiguous: the parser produces a
// `*syntax.WithSpec` whose `WithStmt`s carry an opaque string literal for the
// attribute value. This package walks that AST and produces a typed
// AttributeSet covering `encode`, `variant`, `extension`, `display`, and
// `optional` attributes, with target-element selectors resolved against the
// owning declaration.
//
// We deliberately accept (and surface as diagnostics) anything we don't
// understand rather than rejecting it, so unknown vendor extensions continue
// to round-trip through the formatter without lossy edits.
package attr

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nokia/ntt/ttcn3/syntax"
)

// Kind classifies the attribute keyword (`encode`, `variant`, ...).
type Kind int

const (
	// Unknown marks attributes whose keyword the parser captured but we
	// haven't classified. Callers should round-trip these unchanged.
	Unknown Kind = iota
	// Encode is `with { encode "..." }`. Picks the codec for the target.
	Encode
	// Variant is `with { variant "..." }`. Carries codec-specific
	// per-field options (Annex E.1.5 etc).
	Variant
	// Extension is `with { extension "..." }`. Vendor / Titan-specific
	// extensions (e.g. `prototype(...)`, `transparent`).
	Extension
	// Display is `with { display "..." }`. Affects logging only.
	Display
	// Optional is `with { optional "implicit omit" }` etc.
	Optional
	// ErrorBehavior, PrintingType and friends from Annex E.
	ErrorBehavior
	// AnyOther is reserved for keywords we recognise but haven't given
	// a dedicated kind yet. Treated like Unknown for codec purposes.
	AnyOther
)

// String renders a Kind back to its TTCN-3 keyword.
func (k Kind) String() string {
	switch k {
	case Encode:
		return "encode"
	case Variant:
		return "variant"
	case Extension:
		return "extension"
	case Display:
		return "display"
	case Optional:
		return "optional"
	case ErrorBehavior:
		return "errorbehavior"
	case AnyOther:
		return "anyother"
	}
	return "unknown"
}

// Attribute is a single parsed `with`-clause entry. Selectors are the
// `(field.subfield, all, ...)` qualifiers that scope the attribute to a
// subset of the declaration's children; an empty slice means the attribute
// applies to the declaration as a whole.
type Attribute struct {
	Kind      Kind
	Keyword   string   // Verbatim keyword from source, useful for Unknown.
	Override  bool     // True if the clause was tagged `override`.
	Selectors []string // e.g. "field.sub", "all", "@local", "encode 'RAW'".
	Value     string   // The string literal contents, unquoted.
	Span      syntax.Span
}

// AttributeSet is the result of analysing one `WithSpec`. Attributes are
// returned in source order so the formatter can round-trip them.
type AttributeSet struct {
	Attributes []Attribute
}

// EncodingName returns the codec name selected by the topmost `encode`
// attribute, e.g. "RAW", "JSON", "BER". The Annex-E syntax permits a single
// string with surrounding quotes; we strip them here.
func (s *AttributeSet) EncodingName() string {
	for i := len(s.Attributes) - 1; i >= 0; i-- {
		if s.Attributes[i].Kind == Encode {
			return s.Attributes[i].Value
		}
	}
	return ""
}

// Variants returns every `variant` attribute, in source order. Each one is a
// codec-specific directive; the codec packages turn the value strings into
// concrete encoding plans.
func (s *AttributeSet) Variants() []Attribute {
	var out []Attribute
	for _, a := range s.Attributes {
		if a.Kind == Variant {
			out = append(out, a)
		}
	}
	return out
}

// Extensions returns every `extension` attribute, in source order.
func (s *AttributeSet) Extensions() []Attribute {
	var out []Attribute
	for _, a := range s.Attributes {
		if a.Kind == Extension {
			out = append(out, a)
		}
	}
	return out
}

// HasOptionalImplicitOmit reports whether the set carries the standard
// `with { optional "implicit omit" }` clause, which changes the default
// handling of absent optional fields during encoding.
func (s *AttributeSet) HasOptionalImplicitOmit() bool {
	for _, a := range s.Attributes {
		if a.Kind != Optional {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(a.Value), "implicit omit") {
			return true
		}
	}
	return false
}

// Diagnostic carries a position-tagged note from the parser. We use a small
// dedicated type instead of reusing the semantic package's Diagnostic so this
// package stays at the bottom of the import graph.
type Diagnostic struct {
	Code    string
	Message string
	Span    syntax.Span
}

// Parse walks ws and returns the typed attribute set plus any diagnostics
// raised along the way. ws may be nil, in which case an empty set is
// returned (the common case for declarations without a `with` clause).
func Parse(ws *syntax.WithSpec) (*AttributeSet, []Diagnostic) {
	out := &AttributeSet{}
	var diags []Diagnostic
	if ws == nil {
		return out, nil
	}
	for _, stmt := range ws.List {
		if stmt == nil {
			continue
		}
		a, ds := parseStmt(stmt)
		out.Attributes = append(out.Attributes, a)
		diags = append(diags, ds...)
	}
	return out, diags
}

func parseStmt(stmt *syntax.WithStmt) (Attribute, []Diagnostic) {
	var (
		diags []Diagnostic
		a     = Attribute{
			Keyword:  stmt.KindTok.String(),
			Override: stmt.Override != nil,
			Span:     syntax.SpanOf(stmt),
		}
	)
	a.Kind = classify(stmt.KindTok.Kind())
	a.Selectors = selectorTexts(stmt.List)

	// Only `variant` accepts the encoding-related dot notation
	// (`variant "Codec"."Rule"`); every other attribute keyword
	// must be a single string literal per TTCN-3 27.2
	// restriction c, but the string content itself may contain
	// dots (paths, qualified names, ...).
	if _, ok := stmt.Value.(*syntax.ValueLiteral); ok {
		// Single string literal: accept verbatim, the
		// content (including dots) is part of the value.
		val, _ := plainStringLiteral(stmt.Value)
		a.Value = val
		return a, diags
	}
	val, ok := stringLiteralValue(stmt.Value)
	if !ok || (a.Kind != Variant && containsDot(val)) {
		diags = append(diags, Diagnostic{
			Code:    "attr.non-literal-value",
			Message: fmt.Sprintf("expected a string literal for `%s`", a.Keyword),
			Span:    syntax.SpanOf(stmt.Value),
		})
	}
	a.Value = val
	return a, diags
}

func containsDot(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return true
		}
	}
	return false
}

// plainStringLiteral is the strict version of stringLiteralValue:
// it only accepts a single ValueLiteral and rejects the dot-notation
// composite forms.
func plainStringLiteral(e syntax.Expr) (string, bool) {
	lit, ok := e.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil || lit.Tok.Kind() != syntax.STRING {
		return "", false
	}
	return unquoteAttrString(lit.Tok.String()), true
}

func classify(k syntax.Kind) Kind {
	switch k {
	case syntax.ENCODE:
		return Encode
	case syntax.VARIANT:
		return Variant
	case syntax.EXTENSION:
		return Extension
	case syntax.DISPLAY:
		return Display
	case syntax.OPTIONAL:
		return Optional
	}
	// Annex E lists several other keywords that the parser does not
	// surface as distinct tokens (errorbehavior, printing, ...). They
	// come through as IDENT and we recognise them by spelling later.
	return Unknown
}

// stringLiteralValue extracts the value of a single string literal,
// stripping surrounding quotes. Composite forms used by Annex E are
// also accepted:
//
//   - `variant "Codec1"."Rule1"` parses as a SelectorExpr whose Sel
//     side is itself a string literal; we join the two with a dot.
//   - `variant ("Codec1", "Codec2") "Rule"` shows up as a BinaryExpr
//     after the selector parser; we render it as `Codec1.Codec2.Rule`.
//
// Anything else returns false so callers can flag a diagnostic.
func stringLiteralValue(e syntax.Expr) (string, bool) {
	if e == nil {
		return "", false
	}
	if lit, ok := e.(*syntax.ValueLiteral); ok {
		if lit == nil || lit.Tok == nil || lit.Tok.Kind() != syntax.STRING {
			return "", false
		}
		return unquoteAttrString(lit.Tok.String()), true
	}
	if sel, ok := e.(*syntax.SelectorExpr); ok {
		head, lok := stringLiteralValue(sel.X)
		if !lok {
			return "", false
		}
		// The selector side may itself be a string literal
		// (encoding-related variant), or an identifier (named
		// attribute key). Accept both.
		if tail, ok := stringLiteralValue(sel.Sel); ok {
			return head + "." + tail, true
		}
		if id, ok := sel.Sel.(*syntax.Ident); ok && id.Tok != nil {
			return head + "." + id.Tok.String(), true
		}
		return "", false
	}
	// {"Codec1","Codec2"} - multi-encoding variant selector
	// (Annex E.2.3). Render as a comma-separated list so the
	// dot-notation rule can still split on dots.
	if cl, ok := e.(*syntax.CompositeLiteral); ok {
		var parts []string
		for _, elt := range cl.List {
			if elt == nil {
				continue
			}
			p, ok := stringLiteralValue(elt)
			if !ok {
				return "", false
			}
			parts = append(parts, p)
		}
		if len(parts) == 0 {
			return "", false
		}
		return "{" + strings.Join(parts, ",") + "}", true
	}
	return "", false
}

// unquoteAttrString strips the surrounding TTCN-3 string-literal quotes and
// collapses doubled quotes (`""` -> `"`). It deliberately does not interpret
// any other escape sequences because TTCN-3 string literals don't use them.
func unquoteAttrString(s string) string {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	inner := s[1 : len(s)-1]
	if !strings.Contains(inner, `""`) {
		return inner
	}
	return strings.ReplaceAll(inner, `""`, `"`)
}

// selectorTexts renders the `(...)` selector list into stable strings. The
// concrete syntax mixes identifiers, qualified names and a few keywords
// (`all`, `local`, ...) so we just print whatever the parser captured.
func selectorTexts(list []syntax.Expr) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, exprText(e))
	}
	// We sort selectors so callers can compare sets cheaply without
	// worrying about author-driven ordering. The original order is still
	// available via the AST if a tool needs it.
	sort.Strings(out)
	return out
}

// exprText returns the source-level text of an attribute selector. The
// selectors are intentionally tiny grammars (`x`, `x.y`, `all`, `local`,
// `encode 'RAW'`) so a hand-written walker is enough.
func exprText(e syntax.Expr) string {
	switch n := e.(type) {
	case nil:
		return ""
	case *syntax.Ident:
		return n.String()
	case *syntax.SelectorExpr:
		return exprText(n.X) + "." + exprText(n.Sel)
	case *syntax.ValueLiteral:
		return unquoteAttrString(n.Tok.String())
	}
	// Fallback: render whatever tokens the parser collected. This keeps
	// us robust against new selector forms without crashing.
	if e != nil {
		if tok := e.FirstTok(); tok != nil {
			return tok.String()
		}
	}
	return ""
}
