package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A local definition is visible in its own scope unit and the ones it
// contains only (5.2, 19.2, 19.3).
func TestScopeVisibilityRules(t *testing.T) {
	cases := []struct {
		name, defs string
		want       bool
	}{
		{"constant of another function", `
			function f_a() { const integer c_a := 1; }
			function f_b() return integer { return c_a; }`, true},
		{"variable of a test case in a component", `
			type component D { var integer vc_x := v_tc; }
			testcase TC() runs on C { var integer v_tc := 1; }`, true},
		{"if header variable after the statement", `
			testcase TC() runs on C {
				var integer v_a := 0;
				if (var integer v_x := 1; v_x > 0) { v_a := v_x; }
				v_a := v_x;
			}`, true},
		{"select union header variable after the statement", `
			type union U { integer a, boolean b }
			testcase TC() runs on C {
				select union (var U v_u := { a := 1 }; v_u) { case (a) { } }
				v_u := { b := true };
			}`, true},
		{"parameter of another function", `
			function f_a(integer p_x) { }
			function f_b() return integer { return p_x; }`, true},

		{"own variable", `
			testcase TC() runs on C { var integer v_x := 1; v_x := v_x + 1; }`, false},
		{"variable of an enclosing block", `
			testcase TC() runs on C { var integer v_x := 1; if (true) { v_x := 2; } }`, false},
		{"field named like another function's variable", `
			type record R { integer v_x }
			function f_a() { var integer v_x := 1; }
			function f_b() { var R r := { v_x := 2 }; r.v_x := 3; }`, false},
		{"named parameter like another function's variable", `
			function f_a(integer v_x) { }
			function f_b() { var integer v_y := 1; f_a(v_x := v_y); }`, false},
		{"union alternative like another function's variable", `
			type union U { integer a, boolean b }
			function f_a() { var integer a := 1; }
			function f_b() { var U v_u := { a := 1 }; select union (v_u) { case (a) { } } }`, false},
		{"module constant of the name", `
			const integer c_a := 2;
			function f_a() { const integer c_a := 1; }
			function f_b() return integer { return c_a; }`, false},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C {}
			`+c.defs+`
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		if has := slices.Contains(got, "local-definition-not-visible"); has != c.want {
			t.Errorf("%s: local-definition-not-visible %v, want %v (%v)", c.name, has, c.want, got)
		}
	}
}
