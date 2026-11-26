package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A redirection stores into a variable, not a template definition or a
// constant (15.3 a, 22.2.2).
func TestRedirectTargetRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"field of a template definition", "p.receive(mw_t) -> value mw_t.f1;", "redirect-target-not-variable"},
		{"local template", "template R mw_l := ?; p.receive(R:?) -> value mw_l;", "redirect-target-not-variable"},
		{"constant", "p.receive(R:?) -> value c_r;", "redirect-target-not-variable"},
		{"field assignment to a template field", "p.receive(R:?) -> value (mw_t.f1 := f1);", "redirect-target-not-variable"},

		{"variable", "var R v_r; p.receive(mw_t) -> value v_r;", ""},
		{"field of a variable", "var R v_r; p.receive(mw_t) -> value v_r.f1;", ""},
		{"template variable", "var template R v_t; p.receive(mw_t) -> value v_t;", ""},
		{"field assignment", "var integer v_i; p.receive(R:?) -> value (v_i := f1);", ""},
		{"sender into a variable", "var C v_c; p.receive(mw_t) -> sender v_c;", ""},
	}
	ruleCodes := []string{"redirect-target-not-variable"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer f1, charstring f2 }
			type port P message { inout R }
			type component C { port P p }
			template R mw_t := { f1 := ?, f2 := ? }
			const R c_r := { f1 := 1, f2 := "a" }
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
