package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Module parameter defaults (8.2.1), constants of type definitions (10),
// types of sent values (6.3.4) and keywords of table A.3 (A.1.5.0).
func TestDefinitionValueRules(t *testing.T) {
	cases := []struct {
		name, defs, stmt, want string
	}{
		{"modulepar default calling a runs on function", "modulepar integer mp := f_comp();", "", "modulepar-component-dependent"},
		{"function result in a type constraint", "const integer c_max := f_max(); type integer T (0 .. c_max);", "", "type-constant-not-compile-time"},
		{"value of a compatible but unlisted type sent", "const R2 c_r2 := { f := 1 };", "p.send(c_r2);", "send-type-not-listed"},
		{"keyword of table A.3 as a variable name", "", "var integer nowait := 1;", "keyword-as-identifier"},

		{"modulepar default calling a plain function", "modulepar integer mp := f_max();", "", ""},
		{"literal constant in a type constraint", "const integer c_max := 10; type integer T (0 .. c_max);", "", ""},
		{"value of a listed type sent", "const R1 c_r1 := { f := 1 };", "p.send(c_r1);", ""},
	}
	ruleCodes := []string{"modulepar-component-dependent", "type-constant-not-compile-time", "send-type-not-listed", "keyword-as-identifier"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R1 { integer f }
			type record R2 { integer f }
			type port P message { inout R1 }
			type component C { port P p; var integer v_comp := 1 }
			function f_comp() runs on C return integer { return v_comp }
			function f_max() return integer { return 10 }
			`+c.defs+`
			testcase TC() runs on C {
				`+c.stmt+`
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var hit []string
		for _, code := range got {
			if slices.Contains(ruleCodes, code) {
				hit = append(hit, code)
			}
		}
		switch {
		case c.want == "" && len(hit) > 0:
			t.Errorf("%s: unexpected %v", c.name, hit)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}
