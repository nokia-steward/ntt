package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestPatternMetacharRules(t *testing.T) {
	for _, tt := range []struct {
		name, decls, code string
		reject            bool
	}{
		{"spaced reference", `template charstring t := pattern "x{ c_ref }";`, "pattern-metachar-whitespace", true},
		{"spaced quadruple", `template universal charstring t := pattern "\q{0, 0, 1, 113}";`, "pattern-metachar-whitespace", false},
		{"spaced repetition", `template charstring t := pattern "a#(1, 2)";`, "pattern-metachar-whitespace", false},
		{"type reference", `template universal charstring t := pattern "x\N{universal charstring}";`, "pattern-metachar-whitespace", false},
		{"plain metacharacters", `template charstring t := pattern "x{c_ref}\q{0,0,1,113}a#(1,2)[a b]\{ x \}";`, "pattern-metachar-whitespace", false},
		{"universal set in a charstring template", `template charstring t := pattern "\N{universal charstring}";`, "pattern-universal-set-in-charstring", true},
		{"universal set in a universal charstring template", `template universal charstring t := pattern "\N{universal charstring}";`, "pattern-universal-set-in-charstring", false},
		{"reference to ?", `template charstring a := ?; template charstring t := pattern a;`, "pattern-reference-not-specific", true},
		{"reference to a value list", `template charstring a := ("a", "b"); template charstring t := pattern a;`, "pattern-reference-not-specific", true},
		{"reference to a pattern", `template charstring a := pattern "a*"; template charstring t := pattern a;`, "pattern-reference-not-specific", false},
		{"reference to a value", `template charstring a := "ab"; template charstring t := pattern a;`, "pattern-reference-not-specific", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tree := parse(t, "module M { const charstring c_ref := \"ab\"; "+tt.decls+" }")
			got := containsCode(NewAnalyzer(&ttcn3.DB{}).Analyze(tree), tt.code)
			if got != tt.reject {
				t.Errorf("%s reported: %v, want %v", tt.code, got, tt.reject)
			}
		})
	}
}
