package semantic

import (
	"slices"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// regexp takes character strings, a specific inpar and an expression of
// specific values or patterns (C.4.1); a charstring inpar with a universal
// charstring expression is valid (ETSI, CR 6424).
func TestRegexpRules(t *testing.T) {
	cases := []struct {
		name, call, want string
	}{
		{"octetstring inpar", `regexp(v_oct, charstring:"(?)?", 0)`, "regexp-not-a-character-string"},
		{"integer expression", `regexp(v_str, 1, 0)`, "regexp-not-a-character-string"},
		{"AnyValue inpar", `regexp(charstring:?, charstring:"(?+)", 0)`, "regexp-inpar-not-specific"},
		{"inpar template of a value list", `regexp(t_list, charstring:"(?+)", 0)`, "regexp-inpar-not-specific"},
		{"value list expression", `regexp(v_str, charstring:("(abc)", "(abd)"), 0)`, "regexp-expression-mechanism"},

		{"universal charstring expression", `regexp(v_str, universal charstring:"?+(text)?+", 0)`, ""},
		{"charstrings", `regexp(v_str, charstring:"?+(text)?+", 0)`, ""},
		{"universal charstring inpar", `regexp(v_ustr, universal charstring:"?+(text)?+", 0)`, ""},
		{"universal charstring inpar, charstring expression", `regexp(v_ustr, charstring:"?+(text)?+", 0)`, ""},
		{"pattern expression", `regexp(v_str, pattern "?+(text)?+", 0)`, ""},
		{"specific inpar template", `regexp(t_str, charstring:"(?+)", 0)`, ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C {}
			template charstring t_list := ("a", "b");
			template charstring t_str := "abc";
			testcase TC() runs on C {
				var octetstring v_oct := 'ABCD'O;
				var charstring v_str := "a text";
				var universal charstring v_ustr := "a text";
				var charstring v_res := `+c.call+`;
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var re []string
		for _, code := range got {
			if strings.HasPrefix(code, "regexp-") {
				re = append(re, code)
			}
		}
		switch {
		case c.want == "" && len(re) > 0:
			t.Errorf("%s: unexpected %v", c.name, re)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}
