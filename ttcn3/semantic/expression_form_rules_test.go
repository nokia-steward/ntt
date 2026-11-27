package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Forms of expressions and operands (6.2.15.3, 7.1.3, 15.9, A.1.6.1.3,
// A.1.6.8.3, B.1.4.2).
func TestExpressionFormRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"ifpresent on a mandatory field", "var template R v_t := { f1 := 1 ifpresent, f2 := omit };", "ifpresent-mandatory-field"},
		{"unmap of a map constant", "unmap(c_map, \"a\");", "unmap-not-variable"},
		{"record matched against an integer", "if (match(c_r, c_i)) {}", "match-type-incompatible"},
		{"compound select header", "select ({1, 2}) { case (c_list) {} }", "select-compound-header"},
		{"constant after =>", "var R v_r := v_pdu.payload => c_r;", "decode-target-not-type"},
		{"enumerated types that do not merge", "var E1 v_a := Mon; var E2 v_b := Mon; if (v_a == v_b) {}", "enum-merge-inconsistent"},
		{"template for a value parameter by name", "var integer v := f_a(p_x := ?);", "actual-parameter-restriction-violation"},

		{"ifpresent on an optional field", "var template R v_t := { f1 := 1, f2 := 2 ifpresent };", ""},
		{"unmap of a map variable", "var MyMap v_map := { [\"a\"] := 1 }; unmap(v_map, \"a\");", ""},
		{"enumerated types that merge", "var E1 v_a := Mon; var E3 v_b := Mon; if (v_a == v_b) {}", ""},
		{"value for a value parameter by name", "var integer v := f_a(p_x := 1);", ""},
	}
	ruleCodes := []string{"ifpresent-mandatory-field", "unmap-not-variable", "match-type-incompatible", "select-compound-header", "decode-target-not-type", "enum-merge-inconsistent", "actual-parameter-restriction-violation"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer f1, integer f2 optional }
			type record PDU { octetstring payload }
			type record of integer RoI;
			type map from charstring to integer MyMap;
			type enumerated E1 { Mon, Tue }
			type enumerated E2 { Tue, Mon }
			type enumerated E3 { Mon, Tue, Wed }
			const MyMap c_map := { ["a"] := 1 };
			const R c_r := { f1 := 1, f2 := omit };
			const integer c_i := 1;
			const RoI c_list := { 1, 2 };
			function f_a(in integer p_x) return integer { return p_x }
			type component C {}
			testcase TC() runs on C {
				var PDU v_pdu := { payload := '00'O };
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
