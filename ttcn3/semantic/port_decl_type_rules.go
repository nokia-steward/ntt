// port_decl_type_rules.go enforces ETSI ES 201 873-1 clause 6.2.9
// restrictions on the type identifiers that may appear in a port
// type declaration:
//
//   - Restriction d: formal parameters of `map param (...)` and
//     `unmap param (...)` clauses shall be value parameters of a
//     DATA type. Component, port, timer and default references
//     are forbidden.
//   - Restriction e: the in / out / inout type list of a message
//     port shall reference DATA types only.
//
// We resolve a type identifier through one level of alias (the
// SubTypeDecl form `type default X;` / `type timer X;` etc.), then
// reject when the resolved kind lands in the non-data set.
package semantic

import (
	"fmt"

	"github.com/nokia/ntt/ttcn3/syntax"
)

func (a *Analyzer) checkPortDeclTypeRules(mod *syntax.Module) []Diagnostic {
	components := collectComponentTypeNames(mod)
	ports := collectPortTypeNames(mod)
	aliases := collectTypeAliasKinds(mod)
	var diags []Diagnostic
	classify := func(ty string) string {
		switch ty {
		case "":
			return ""
		case "timer", "default":
			return ty
		}
		if components[ty] {
			return "component"
		}
		if ports[ty] {
			return "port"
		}
		if kind, ok := aliases[ty]; ok {
			return kind
		}
		return ""
	}
	syntax.Inspect(mod, func(n syntax.Node) bool {
		pd, ok := n.(*syntax.PortTypeDecl)
		if !ok || pd == nil {
			return true
		}
		portKind := ""
		if pd.KindTok != nil {
			portKind = pd.KindTok.String()
		}
		for _, attr := range pd.Attrs {
			switch a := attr.(type) {
			case *syntax.PortAttribute:
				if portKind != "message" {
					continue
				}
				if a.KindTok == nil {
					continue
				}
				switch a.KindTok.String() {
				case "in", "out", "inout":
				default:
					continue
				}
				for _, te := range a.Types {
					name := identNameForPortRule(te)
					kind := classify(name)
					if kind == "" {
						continue
					}
					diags = append(diags, Diagnostic{
						Code:     "port-message-non-data-type",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"message port %q lists %s type %q in its %s clause; expected a data type (ETSI 6.2.9 e)",
							portTypeName(pd), kind, name, a.KindTok.String()),
						Node: te,
						Span: syntax.SpanOf(te),
					})
				}
			case *syntax.PortMapAttribute:
				if a.Params == nil {
					continue
				}
				op := ""
				if a.MapTok != nil {
					op = a.MapTok.String()
				}
				for _, fp := range a.Params.List {
					if fp == nil || fp.Type == nil {
						continue
					}
					name := identNameForPortRule(fp.Type)
					kind := classify(name)
					if kind == "" {
						continue
					}
					diags = append(diags, Diagnostic{
						Code:     "port-mapparam-non-data-type",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"%s param of port %q binds %s type %q; expected a data type (ETSI 6.2.9 d)",
							op, portTypeName(pd), kind, name),
						Node: fp.Type,
						Span: syntax.SpanOf(fp.Type),
					})
				}
			}
		}
		return true
	})
	return diags
}

func portTypeName(pd *syntax.PortTypeDecl) string {
	if pd == nil || pd.Name == nil {
		return "<?>"
	}
	return pd.Name.String()
}

// identNameForPortRule reduces an Expr/Node to its top-level
// identifier name. We accept Ident directly and RefSpec(Ident)
// (the parser sometimes wraps formal-parameter types in a
// RefSpec). Returns "" for shapes we don't recognise (parameterised
// types, etc.), so the caller silently skips.
func identNameForPortRule(n syntax.Node) string {
	switch x := n.(type) {
	case *syntax.Ident:
		if x == nil || x.Tok == nil {
			return ""
		}
		return x.Tok.String()
	case *syntax.RefSpec:
		if x == nil {
			return ""
		}
		if id, ok := x.X.(*syntax.Ident); ok && id != nil && id.Tok != nil {
			return id.Tok.String()
		}
	}
	return ""
}

// collectTypeAliasKinds walks every `type <X> Y;` (SubTypeDecl
// without a body) and maps Y -> kind, where kind is the head of
// the alias chain: "timer", "default", "component", "port", or
// "" for a data alias we don't want to flag. We only follow one
// level of aliasing; nested chains land as the leaf kind because
// every intermediate alias is itself indexed.
func collectTypeAliasKinds(mod *syntax.Module) map[string]string {
	out := map[string]string{}
	// First pass: direct aliases to keyword kinds.
	syntax.Inspect(mod, func(n syntax.Node) bool {
		std, ok := n.(*syntax.SubTypeDecl)
		if !ok || std == nil || std.Field == nil {
			return true
		}
		name := ""
		if std.Field.Name != nil {
			name = std.Field.Name.String()
		}
		if name == "" {
			return true
		}
		base := ""
		if rs, ok := std.Field.Type.(*syntax.RefSpec); ok && rs != nil {
			if id, ok := rs.X.(*syntax.Ident); ok && id != nil && id.Tok != nil {
				base = id.Tok.String()
			}
		}
		switch base {
		case "timer", "default":
			out[name] = base
		}
		return true
	})
	// Second pass: aliases to component / port (need the
	// containing-module tables built first).
	components := collectComponentTypeNames(mod)
	ports := collectPortTypeNames(mod)
	syntax.Inspect(mod, func(n syntax.Node) bool {
		std, ok := n.(*syntax.SubTypeDecl)
		if !ok || std == nil || std.Field == nil {
			return true
		}
		name := ""
		if std.Field.Name != nil {
			name = std.Field.Name.String()
		}
		if name == "" || out[name] != "" {
			return true
		}
		base := ""
		if rs, ok := std.Field.Type.(*syntax.RefSpec); ok && rs != nil {
			if id, ok := rs.X.(*syntax.Ident); ok && id != nil && id.Tok != nil {
				base = id.Tok.String()
			}
		}
		switch {
		case components[base]:
			out[name] = "component"
		case ports[base]:
			out[name] = "port"
		}
		return true
	})
	return out
}
