package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Formal parameters, component type members and enumerated values follow
// the uniqueness of identifiers (5.2.2, 6.2.4, 6.2.10.1, 16.3).
func TestDefinitionUniquenessRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"duplicate formal parameter", `testcase TC(integer p_a, integer p_a) runs on C {}`, "duplicate-identifier-in-scope"},
		{"duplicate template parameter", `template integer t(integer p_a, integer p_a) := p_a;`, "duplicate-identifier-in-scope"},
		{"parameter named like a module constant", `const integer c_a := 1; function f(integer c_a) {}`, "identifier-shadows-module-def"},
		{"duplicate port", `type component D { port P p; port P p }`, "duplicate-identifier-in-scope"},
		{"port and variable of a name", `type component D { port P p; var integer p }`, "duplicate-identifier-in-scope"},
		{"constant named like an enumerated value", `const integer e_a := 1;`, "enum-value-identifier-reused"},
		{"local variable named like an enumerated value", `function f() { var integer e_b := 1; }`, "enum-value-identifier-reused"},
		{"parameter named like an enumerated value", `function f(integer e_a) {}`, "enum-value-identifier-reused"},

		{"distinct parameters", `testcase TC(integer p_a, integer p_b) runs on C {}`, ""},
		{"same parameter name in two functions", `function f(integer p_a) {} function g(integer p_a) {}`, ""},
		{"distinct ports", `type component D { port P p1; port P p2 }`, ""},
		{"field named like an enumerated value", `type record R { integer e_a }`, ""},
		{"enumerated value reused in another enumerated type", `type enumerated E2 { e_a, e_c }`, ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port P message { inout integer }
			type component C {}
			type enumerated E { e_a, e_b }
			`+c.defs+`
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		ruleCodes := []string{"duplicate-identifier-in-scope", "identifier-shadows-module-def", "enum-value-identifier-reused"}
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
