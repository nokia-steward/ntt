package semantic

import (
	"fmt"
	"strings"

	"github.com/nokia/ntt/ttcn3/attr"
	"github.com/nokia/ntt/ttcn3/syntax"
)

// knownEncodings is the canonical Annex-E codec list. Vendor codecs (e.g.
// Titan's "RAW", "TEXT", "JSON", "XML", "BER", "OER", "PER") are listed
// case-insensitively. The analyzer also queries the codec registry at
// runtime so user-installed codecs become "known" without editing this
// table. Unknown encodings still emit a warning so `with { encode
// "MY_CUSTOM" }` compiles but flags an obvious typo.
var knownEncodings = map[string]bool{
	"raw":           true,
	"text":          true,
	"json":          true,
	"xml":           true,
	"xer":           true,
	"ber":           true,
	"cer":           true,
	"der":           true,
	"oer":           true,
	"per":           true,
	"per-aligned":   true,
	"per_aligned":   true,
	"per-unaligned": true,
	"per_unaligned": true,
}

// checkAttributes walks every `with { ... }` clause in the module, parses
// it through the attr package, and surfaces any structural diagnostics
// raised by the parser plus a soft warning for unknown codec names.
func (a *Analyzer) checkAttributes(mod *syntax.Module) []Diagnostic {
	var out []Diagnostic
	mod.Inspect(func(n syntax.Node) bool {
		ws, ok := n.(*syntax.WithSpec)
		if !ok {
			return true
		}
		set, diags := attr.Parse(ws)
		for _, d := range diags {
			out = append(out, Diagnostic{
				Code:     d.Code,
				Severity: SeverityError,
				Message:  d.Message,
				Span:     d.Span,
			})
		}
		declaredEncodings := map[string]bool{}
		for _, at := range set.Attributes {
			if at.Kind != attr.Encode {
				continue
			}
			value := strings.TrimSpace(at.Value)
			if value == "" {
				out = append(out, Diagnostic{
					Code:     "attr.empty-encoding",
					Severity: SeverityError,
					Message:  "`encode` clause must name a codec",
					Span:     at.Span,
				})
				continue
			}
			declaredEncodings[value] = true
			if !knownEncodings[strings.ToLower(value)] {
				out = append(out, Diagnostic{
					Code:     "attr.unknown-encoding",
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("unknown encoding %q (no codec backend registered)", value),
					Span:     at.Span,
				})
			}
		}
		// Cross-check: a `variant` attribute that uses dot
		// notation must reference an encoding that has been
		// declared via `encode` in the same `with { ... }`
		// scope (ETSI 27.5 restriction a).  Skip the cross-
		// check entirely when no `encode` declarations live
		// in this scope - a subtype that only overwrites a
		// variant (e.g. Sem_27010202_002's `type Int Int3 with
		// { variant "CodecB"."Rule4"; }`) inherits the codec
		// list from its base and the suite expects no error.
		if len(declaredEncodings) > 0 {
			for _, at := range set.Attributes {
				if at.Kind != attr.Variant {
					continue
				}
				prefix := variantEncodingPrefix(at.Value)
				if prefix == "" {
					// With several encodings in force, a plain
					// variant is ambiguous: ETSI 27.5 requires the
					// encoding reference (`variant "Codec"."Rule"`).
					if len(declaredEncodings) >= 2 {
						out = append(out, Diagnostic{
							Code:     "attr.variant-ambiguous",
							Severity: SeverityError,
							Message:  "variant without an encoding reference is not allowed when several encodings apply (ETSI 27.5)",
							Span:     at.Span,
						})
					}
					continue
				}
				for _, codec := range strings.Split(prefix, ",") {
					codec = strings.TrimSpace(codec)
					if codec == "" {
						continue
					}
					if declaredEncodings[codec] {
						continue
					}
					out = append(out, Diagnostic{
						Code:     "attr.variant-unknown-encoding",
						Severity: SeverityError,
						Message: fmt.Sprintf(
							"variant references encoding %q not declared in the same `with` scope (ETSI 27.5 a)",
							codec),
						Span: at.Span,
					})
				}
			}
		}
		return true
	})
	return out
}

// variantEncodingPrefix returns the codec selector that prefixes a
// variant value, or "" when the variant is plain. The two shapes
// handled are:
//
//   - `"Codec"."Rule"` -> parser surfaces this as "Codec.Rule".
//   - `{"Codec1","Codec2"}."Rule"` -> parser surfaces as
//     "{Codec1,Codec2}.Rule"; we strip the braces and return
//     "Codec1,Codec2" so the caller can split.
//
// Any other shape (no dot, no brace) returns the empty string,
// signalling that there is no encoding cross-check to do.
func variantEncodingPrefix(value string) string {
	dot := strings.LastIndex(value, ".")
	if dot <= 0 {
		return ""
	}
	prefix := value[:dot]
	if strings.HasPrefix(prefix, "{") && strings.HasSuffix(prefix, "}") {
		return prefix[1 : len(prefix)-1]
	}
	return prefix
}
