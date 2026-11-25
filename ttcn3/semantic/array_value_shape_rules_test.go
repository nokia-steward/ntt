package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Each assigned dimension of a multi-dimensional array is a brace-enclosed
// list (6.2.7); a timer default is non-negative, also through constants
// (12); send takes a specific value (22.2.1) and raise a template(value)
// (22.3.5 f), also through template definitions.
func TestArrayValueShapeRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"flat list into a 2-dimensional array", "v_a := { 1, 2, 3, 4 };", "array-dimension-not-braced"},
		{"flat list initialising a 2-dimensional array", "var integer v_b[2][2] := { 1, 2, 3, 4 };", "array-dimension-not-braced"},
		{"flat row in a 3-dimensional array", "var integer v_b[2][2][2] := { { 1, 2 }, { { 1, 2 }, { 3, 4 } } };", "array-dimension-not-braced"},
		{"negative timer default from a constant", "timer T_t := c_base - 1.0;", "timer-negative-default"},
		{"negative timer default from a constant expression", "timer T_t := -(c_base * 2.0);", "timer-negative-default"},
		{"send of a template with a range", "p.send(mw_range);", "send-template-not-specific"},
		{"send of an inline value list", "p.send(R:{ a := 1, b := (1, 2) });", "send-template-not-specific"},
		{"raise of a value list template", "p_proc.raise(S, mw_exc);", "raise-exception-not-specific"},

		{"braced 2-dimensional array", "v_a := { { 1, 2 }, { 3, 4 } };", ""},
		{"rows from variables", "var integer v_row[2] := { 1, 2 }; v_a := { v_row, v_row };", ""},
		{"1-dimensional array", "var integer v_c[4] := { 1, 2, 3, 4 };", ""},
		{"record of in a 2-dimensional array", "var RI v_d[2][2] := { { { 1 }, { 2 } }, { { 3 }, { 4 } } };", ""},
		{"timer default from a constant", "timer T_t := c_base + 1.0;", ""},
		{"timer default from a variable", "var float v_f := -1.0; timer T_t := v_f;", ""},
		{"send of a specific template", "p.send(mw_specific);", ""},
		{"send with omit", "p.send(R:{ a := 1, b := omit });", ""},
		{"raise of a specific template", "p_proc.raise(S, mw_exc_one);", ""},
	}
	ruleCodes := []string{"array-dimension-not-braced", "timer-negative-default", "send-template-not-specific", "raise-exception-not-specific"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer a, integer b optional }
			type record of integer RI;
			type port P message { inout R }
			signature S() exception(integer);
			type port PP procedure { inout S }
			type component C { port P p; port PP p_proc }
			const float c_base := 0.5;
			template R mw_range := { a := 1, b := (1 .. 5) }
			template R mw_specific := { a := 1, b := 2 }
			template integer mw_exc := (1, 2);
			template integer mw_exc_one := 1;
			testcase TC() runs on C {
				var integer v_a[2][2];
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
