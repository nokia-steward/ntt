package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/attr"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// unicharEncodings catalogues the codec names defined by ETSI
// clause C.5.4 (decvalue_unichar) plus the common ISO charset
// names accepted by Annex E. They are recognised regardless of
// any `encode` attribute in the module.
var unicharEncodings = map[string]bool{
	"utf-8":         true,
	"utf-16":        true,
	"utf-16be":      true,
	"utf-16le":      true,
	"utf-32":        true,
	"utf-32be":      true,
	"utf-32le":      true,
	"ascii":         true,
	"us-ascii":      true,
	"iso-8859-1":    true,
	"iso-8859-15":   true,
	"latin-1":       true,
	"windows-1252":  true,
}

// checkDecodedRedirectRules enforces ETSI 22.2.{2,3} / 22.3.{4,6}:
// in a `-> value (var := @decoded fieldName)` or `-> param
// (var := @decoded fieldName)` redirect, the source field
// MUST be of one of:
//
//   - bitstring
//   - hexstring
//   - octetstring
//   - charstring
//   - universal charstring
//
// Otherwise the runtime cannot interpret it as an encoded
// payload. We approximate "field type" by walking every record /
// struct / union declaration in the module and recording the
// type ident of each field name. If a field's ALL observed types
// across the module are outside the allowed set, any `@decoded
// <thatField>` reference is flagged.
//
// We use a conservative "all observed are bad" rule to avoid
// false positives when the same field name appears in several
// types of mixed legality.
//
// The rule also validates the optional encoding-format argument
// of `@decoded(arg) fieldName`. We only flag the easy cases the
// suite cares about:
//
//   - `@decoded(<integer-typed-var>)` - the format must be a
//     charstring expression, never an integer
//     (NegSem_220202_008 / NegSem_220203_008).
func (a *Analyzer) checkDecodedRedirectRules(mod *syntax.Module) []Diagnostic {
	var diags []Diagnostic
	fieldTypes := collectFieldTypes(mod)
	intVars := collectIntegerLocalVars(mod)
	localEncodings := collectLocalEncodingNames(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		de, ok := n.(*syntax.DecodedExpr)
		if !ok || de == nil || de.X == nil {
			return true
		}
		diags = append(diags, decodedFormatDiags(de, intVars)...)
		diags = append(diags, decodedFormatNameDiags(de, localEncodings)...)
		id, ok := de.X.(*syntax.Ident)
		if !ok || id == nil {
			return true
		}
		fieldName := id.String()
		types, ok := fieldTypes[fieldName]
		if !ok || len(types) == 0 {
			return true
		}
		// ETSI 22.{2,3}: when `@decoded(...)` carries an
		// optional codec / format parameter, the source
		// field MUST be `universal charstring`. Other
		// legal source types (bit / hex / oct / charstring)
		// take no format arg.
		if decodedHasFormatArg(de) {
			allUnichar := true
			for t := range types {
				if t != "universal charstring" {
					allUnichar = false
					break
				}
			}
			if !allUnichar {
				diags = append(diags, Diagnostic{
					Code:     "decoded-format-arg-non-unichar",
					Severity: SeverityError,
					Message: fmt.Sprintf(
						"`@decoded(...) %s`: optional format parameter is only valid when the source field is `universal charstring` (ETSI 22.{2,3})",
						fieldName),
					Node: de,
					Span: syntax.SpanOf(de),
				})
			}
		}
		// `@decoded` is legal as long as at least one
		// observed type is an encoded-payload candidate.
		anyOk := false
		for t := range types {
			if isEncodedPayloadType(t) {
				anyOk = true
				break
			}
		}
		if anyOk {
			return true
		}
		// Build a sorted-ish "observed types" string for
		// the diagnostic; map iteration is fine here.
		observed := ""
		for t := range types {
			if observed != "" {
				observed += ", "
			}
			observed += t
		}
		diags = append(diags, Diagnostic{
			Code:     "decoded-bad-source-type",
			Severity: SeverityError,
			Message: fmt.Sprintf(
				"`@decoded %s`: source field type %s cannot be decoded; expected bitstring/hexstring/octetstring/charstring/universal charstring (ETSI 22.{2,3})",
				fieldName, observed),
			Node: de,
			Span: syntax.SpanOf(de),
		})
		return true
	})
	return diags
}

// decodedFormatDiags inspects the optional `@decoded(arg)`
// format argument. The arg should be a charstring expression
// identifying the codec; passing a value of integer type makes
// no sense and is flagged.
func decodedFormatDiags(
	de *syntax.DecodedExpr,
	intVars map[string]bool,
) []Diagnostic {
	if de == nil || de.Params == nil {
		return nil
	}
	pe, ok := de.Params.(*syntax.ParenExpr)
	if !ok || pe == nil || len(pe.List) == 0 {
		return nil
	}
	first := pe.List[0]
	id, ok := first.(*syntax.Ident)
	if !ok || id == nil || id.Tok == nil {
		return nil
	}
	name := id.String()
	if !intVars[name] {
		return nil
	}
	return []Diagnostic{{
		Code:     "decoded-format-not-charstring",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"`@decoded(%s)`: format argument has integer type; expected a charstring codec identifier (ETSI 22.{2,3})",
			name),
		Node: de,
		Span: syntax.SpanOf(de),
	}}
}

// decodedFormatNameDiags inspects the `@decoded("...")` codec
// string and rejects names that aren't recognised:
//
//   - knownEncodings (the canonical Annex-E codec list)
//   - unicharEncodings (UTF-* / common ISO charsets)
//   - any codec the module declares via `encode "..."`
//
// Names that look like neither are likely typos or placeholders
// (`"proprietary"`) and the spec requires the codec to be
// resolvable at compile time.
func decodedFormatNameDiags(
	de *syntax.DecodedExpr,
	localEncodings map[string]bool,
) []Diagnostic {
	if de == nil || de.Params == nil {
		return nil
	}
	pe, ok := de.Params.(*syntax.ParenExpr)
	if !ok || pe == nil || len(pe.List) == 0 {
		return nil
	}
	first := pe.List[0]
	lit, ok := first.(*syntax.ValueLiteral)
	if !ok || lit == nil || lit.Tok == nil {
		return nil
	}
	raw := lit.Tok.String()
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil
	}
	name := raw[1 : len(raw)-1]
	if name == "" {
		return nil
	}
	lower := strings.ToLower(name)
	if knownEncodings[lower] || unicharEncodings[lower] {
		return nil
	}
	if localEncodings[name] {
		return nil
	}
	return []Diagnostic{{
		Code:     "decoded-unknown-codec",
		Severity: SeverityError,
		Message: fmt.Sprintf(
			"`@decoded(%s)`: codec %q is not declared in the module and is not a known TTCN-3 / unichar codec (ETSI 22.{2,3})",
			raw, name),
		Node: de,
		Span: syntax.SpanOf(de),
	}}
}

// collectLocalEncodingNames walks every `with { ... }` clause
// in the module and gathers the codec names declared via
// `encode "..."`. We pass them through attr.Parse so the
// extraction matches the official attribute grammar.
func collectLocalEncodingNames(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	if mod == nil {
		return out
	}
	mod.Inspect(func(n syntax.Node) bool {
		ws, ok := n.(*syntax.WithSpec)
		if !ok || ws == nil {
			return true
		}
		set, _ := attr.Parse(ws)
		for _, at := range set.Attributes {
			if at.Kind != attr.Encode {
				continue
			}
			name := strings.TrimSpace(at.Value)
			if name != "" {
				out[name] = true
			}
		}
		return true
	})
	return out
}

// decodedHasFormatArg reports whether de carries a non-empty
// `@decoded(args) field` parameter list. Used by the
// universal-charstring-only check.
func decodedHasFormatArg(de *syntax.DecodedExpr) bool {
	if de == nil || de.Params == nil {
		return false
	}
	pe, ok := de.Params.(*syntax.ParenExpr)
	if !ok || pe == nil {
		return false
	}
	return len(pe.List) > 0
}

// collectIntegerLocalVars maps every local `var integer X` (and
// formal-parameter `integer X`) name to true so the @decoded
// format-arg check can recognise when an ident refers to an
// integer-typed value rather than a charstring codec name.
func collectIntegerLocalVars(mod *syntax.Module) map[string]bool {
	out := map[string]bool{}
	if mod == nil {
		return out
	}
	mod.Inspect(func(n syntax.Node) bool {
		vd, ok := n.(*syntax.ValueDecl)
		if !ok || vd == nil {
			return true
		}
		if exprFieldTypeName(vd.Type) != "integer" {
			return true
		}
		for _, d := range vd.Decls {
			if d == nil || d.Name == nil {
				continue
			}
			out[d.Name.String()] = true
		}
		return true
	})
	return out
}

// collectFieldTypes maps a field name to the set of all named
// types observed for that field across struct/list-of decls.
// `record of integer payload` ends up as fieldTypes["payload"]
// = {"record of integer"}. We use a plain string representation
// because the goal is membership testing, not type rebuilding.
func collectFieldTypes(mod *syntax.Module) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	add := func(name, typ string) {
		if name == "" || typ == "" {
			return
		}
		if out[name] == nil {
			out[name] = map[string]bool{}
		}
		out[name][typ] = true
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		switch v := n.(type) {
		case *syntax.StructTypeDecl:
			if v == nil {
				return true
			}
			for _, f := range v.Fields {
				if f == nil || f.Name == nil {
					continue
				}
				add(f.Name.String(), describeFieldType(f.Type))
			}
		case *syntax.SignatureDecl:
			// Procedure-signature parameters are valid
			// targets of `@decoded`. Track their types
			// alongside record/struct fields so the rule
			// catches `getcall/getreply -> param (var :=
			// @decoded p_par)` where p_par is not one of
			// the bit/hex/oct/char-string family.
			if v == nil || v.Params == nil {
				return true
			}
			for _, p := range v.Params.List {
				if p == nil || p.Name == nil {
					continue
				}
				add(p.Name.String(), exprFieldTypeName(p.Type))
			}
		}
		return true
	})
	return out
}

// exprFieldTypeName returns a short label for an Expr used as a
// type reference in a signature parameter list (where the parser
// emits Expr, not TypeSpec). Falls back to identName which gives
// the bare ident for `RoI`, `integer`, etc.
func exprFieldTypeName(e syntax.Expr) string {
	if e == nil {
		return ""
	}
	return identName(e)
}

// describeFieldType returns a short string label for the type
// spec so we can compare it against the "encoded payload" set.
// We only need to disambiguate the five allowed primitive types
// vs. everything else; anything we can't easily classify is
// labelled "<other>" so the rule stays quiet.
func describeFieldType(t syntax.TypeSpec) string {
	if t == nil {
		return ""
	}
	if rs, ok := t.(*syntax.RefSpec); ok && rs != nil && rs.X != nil {
		if id, ok := rs.X.(*syntax.Ident); ok && id != nil && id.Tok != nil {
			return id.String()
		}
	}
	if ls, ok := t.(*syntax.ListSpec); ok && ls != nil {
		return "record of " + describeFieldType(ls.ElemType)
	}
	return "<other>"
}

func isEncodedPayloadType(t string) bool {
	switch t {
	case "bitstring", "hexstring", "octetstring",
		"charstring", "universal charstring":
		return true
	}
	return false
}
