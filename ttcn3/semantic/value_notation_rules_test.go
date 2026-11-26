package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Values of enumerated (6.2.4), record of (6.2.3), union (6.2.5) and
// string element (6.1.1.1) types.
func TestValueNotationRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"value of another enumerated type", "var E2 v_e := e_a;", "enum-value-of-other-type"},
		{"assigned value of another enumerated type", "var E2 v_e; v_e := e_b;", "enum-value-of-other-type"},
		{"value with a list of integers used bare", "var E3 v_e := e_list;", "enum-value-without-integer"},
		{"record of element by name", "var RoI v_r := { f1 := 1 };", "list-element-identifier"},
		{"not used symbol in a union value", "var U v_u := { number := - };", "union-value-not-given"},
		{"omit in a union value", "var U v_u := { number := omit };", "union-value-not-given"},
		{"two bits into a bitstring element", "var bitstring v_b := '010'B; v_b[1] := '11'B;", "string-element-length"},

		{"value with a range used bare", "var E3 v_e := e_rng;", "enum-value-without-integer"},

		{"value with an integer expression", "var E3 v_e := e_expr;", ""},
		{"value of its own type", "var E2 v_e := e_c;", ""},
		{"value with a list of integers and one of them", "var E3 v_e := e_list(7);", ""},
		{"variable of the enumerated type", "var E2 v_a := e_c; var E2 v_e := v_a;", ""},
		{"record of element by index", "var RoI v_r := { [0] := 1 };", ""},
		{"union value", "var U v_u := { number := 1 };", ""},
		{"one bit into a bitstring element", "var bitstring v_b := '010'B; v_b[1] := '1'B;", ""},
		{"value into a record of element", "var RoI v_r := { 1 }; v_r[0] := 2;", ""},
	}
	ruleCodes := []string{"enum-value-of-other-type", "enum-value-without-integer", "list-element-identifier", "union-value-not-given", "string-element-length"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type enumerated E1 { e_a, e_b }
			type enumerated E2 { e_c, e_d }
			type enumerated E3 { e_x, e_list(5, 7 .. 9), e_expr(2 + 2), e_rng(10 .. 12) }
			type record of integer RoI;
			type union U { integer number, charstring string }
			type component C {}
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
