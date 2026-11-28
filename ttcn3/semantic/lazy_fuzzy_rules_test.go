package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Lazy and fuzzy variables and parameters: no out or inout calls in their
// values (11.2 i), @deterministic in deterministic contexts, and then with
// no operations 16.1.4 forbids (11.2 j, 5.4.1.1 n, 5.4.1.2 j, 5.4.2 p).
func TestLazyFuzzyRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"inout call in a lazy initialiser", "testcase TC() runs on C { var integer v_i := 0; var template @lazy integer v_t := f_inc(v_i); }", "lazy-fuzzy-init-out-param"},
		{"fuzzy variable in a receive", "testcase TC() runs on C { var template @fuzzy integer v_t := 1; p.receive(v_t); }", "lazy-fuzzy-not-deterministic"},
		{"lazy parameter in a receive", "function f(template @lazy integer p_t) runs on C { p.receive(p_t); }", "lazy-fuzzy-not-deterministic"},
		{"fuzzy parameter of a global template", "template integer m_v(@fuzzy integer p_x) := p_x;", "lazy-fuzzy-not-deterministic"},
		{"fuzzy variable in an alt guard", "testcase TC() runs on C { var @fuzzy boolean v_b := true; alt { [v_b] p.receive {} } }", "lazy-fuzzy-not-deterministic"},
		{"rnd in a deterministic initialiser", "testcase TC() runs on C { var template @fuzzy @deterministic float v_t := rnd(); }", "deterministic-init-side-effect"},
		{"rnd for a deterministic parameter", "function f_d(in @lazy @deterministic float p_v) return float { return p_v; } testcase TC() runs on C { var float v := f_d(rnd()); }", "deterministic-arg-side-effect"},

		{"deterministic fuzzy variable in a receive", "testcase TC() runs on C { var template @fuzzy @deterministic integer v_t := 1; p.receive(v_t); }", ""},
		{"lazy variable as an index redirection", "testcase TC() runs on C { var @lazy integer v_l; alt { [] any from pa.check(receive(5) -> @index value v_l) {} } }", ""},
		{"fuzzy variable sent", "testcase TC() runs on C { var template @fuzzy integer v_t := 1; p.send(valueof(v_t)); }", ""},
		{"fuzzy parameter of a fuzzy template", "template @fuzzy integer m_v(@fuzzy integer p_x) := p_x;", ""},
		{"in call in a lazy initialiser", "testcase TC() runs on C { var @lazy integer v_t := f_in(1); }", ""},
		{"rnd in a fuzzy initialiser", "testcase TC() runs on C { var @fuzzy float v_f := rnd(); }", ""},
	}
	ruleCodes := []string{"lazy-fuzzy-init-out-param", "lazy-fuzzy-not-deterministic", "deterministic-init-side-effect", "deterministic-arg-side-effect"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port P message { inout integer }
			type component C { port P p; port P pa[2] }
			function f_inc(inout integer p_i) return integer { p_i := p_i + 1; return p_i; }
			function f_in(in integer p_i) return integer { return p_i; }
			`+c.defs+`
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
