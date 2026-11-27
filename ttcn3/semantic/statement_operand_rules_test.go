package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Operands of assignments (19.1.1), log (19.11), deactivate (20.5.3)
// and setverdict (24.2).
func TestStatementOperandRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"omit into a mandatory field", "var R v_r := { a := 1, b := 2 }; v_r.a := omit;", "omit-mandatory-field"},
		{"running of a timer array logged", "timer T_arr[2]; log(T_arr.running);", "log-array-element-missing"},
		{"template as the suspend expression", "var default v_d := activate(a_1()); deactivate(v_d, m_true);", "deactivate-suspend-template"},
		{"function without return as a reason", "setverdict(pass, xf_noReturn(1));", "log-no-return-function"},
		{"function without return logged", "log(xf_noReturn(1));", "log-no-return-function"},

		{"omit into an optional field", "var R v_r := { a := 1, b := 2 }; v_r.b := omit;", ""},
		{"running of a timer array element logged", "timer T_arr[2]; log(T_arr[0].running);", ""},
		{"boolean suspend expression", "var default v_d := activate(a_1()); deactivate(v_d, true);", ""},
		{"function with return as a reason", "setverdict(pass, f_int());", ""},
	}
	ruleCodes := []string{"omit-mandatory-field", "log-array-element-missing", "deactivate-suspend-template", "log-no-return-function"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R { integer a, integer b optional }
			type component C { timer T }
			template boolean m_true := true;
			external function xf_noReturn(integer p_i);
			function f_int() return integer { return 1 }
			altstep a_1() runs on C { [] T.timeout {} }
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
