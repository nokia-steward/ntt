package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Defaults of component type parameters are null, mtc, self or system;
// of port, timer and default type parameters null (5.4.1.1 f, g;
// 5.4.1.2 e).
func TestParamDefaultRules(t *testing.T) {
	cases := []struct {
		name, params string
		want         bool
	}{
		{"component from a call", "C p := f_comp()", true},
		{"component template from a call", "template C p := f_comp()", true},
		{"component created", "C p := C.create", true},
		{"component from a variable", "C p := v_comp", true},
		{"timer from a component timer", "timer p := T", true},
		{"port from a component port", "P p := p_port", true},
		{"default from a variable", "default p := v_def", true},

		{"component null", "C p := null", false},
		{"component mtc", "C p := mtc", false},
		{"component self", "C p := self", false},
		{"component system", "C p := system", false},
		{"component template list", "template C p := (mtc, system)", false},
		{"component template any", "template C p := ?", false},
		{"timer null", "timer p := null", false},
		{"port null", "P p := null", false},
		{"default null", "default p := null", false},
		{"integer from a call", "integer p := f_int()", false},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port P message { inout integer }
			type component C { timer T; port P p_port; var C v_comp; var default v_def }
			function f_comp() return C { return null }
			function f_int() return integer { return 1 }
			function f(`+c.params+`) runs on C {}
		}`)
		got := slices.Contains(codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree)), "param-default-not-special-value")
		if got != c.want {
			t.Errorf("%s: param-default-not-special-value = %v, want %v", c.name, got, c.want)
		}
	}
}
