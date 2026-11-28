package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Module parameters are not used in type definitions (8.2.1), array
// dimensions are not formal parameters of behaviours (6.2.7), constants
// have a value (10) and groups declare no variables (11.1 c).
func TestTypeDefRefRules(t *testing.T) {
	cases := []struct {
		name, defs, want string
	}{
		{"modulepar in a subtype", "type integer MyInt (0 .. mp_max);", "modulepar-in-type-definition"},
		{"modulepar in a field", "type record R { integer f (0 .. mp_max) }", "modulepar-in-type-definition"},
		{"array dimension parameter", "function f(integer p_n) { var integer v[p_n]; }", "array-dim-non-const"},
		{"union of itself and an integer", "type union U { integer a, U b }", ""},
		{"const without a value", "function f() { const integer c_x; }", "const-without-value"},
		{"variable in a group", "group G { var integer v_g; }", "module-variable"},

		{"constant in a subtype", "const integer c_max := 3; type integer MyInt (0 .. c_max);", ""},
		{"field named as a modulepar", "type record R { integer mp_max }", ""},
		{"modulepar in a function", "function f() { var integer v := mp_max; }", ""},
		{"array dimension constant", "const integer c_n := 2; function f() { var integer v[c_n]; }", ""},
		{"const with a value", "function f() { const integer c_x := 1; }", ""},
		{"constant in a group", "group G { const integer c_g := 1; }", ""},
	}
	ruleCodes := []string{"modulepar-in-type-definition", "array-dim-non-const", "const-without-value", "module-variable"}
	for _, c := range cases {
		tree := parse(t, `module M {
			modulepar integer mp_max := 10;
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
