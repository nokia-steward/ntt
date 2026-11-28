package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Operands of equality (7.1.3) and of list concatenation (7.1.2).
func TestOperandTypeRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"behaviour values compared", "var F_t v_a := refers(f_a), v_b := refers(f_a); if (v_a == v_b) {}", "behaviour-type-comparison"},
		{"addresses of a behaviour type compared", "var address v_a := refers(f_a), v_b := refers(f_a); if (v_a != v_b) {}", "address-behaviour-comparison"},
		{"value lists of two root types", "if ({ 1, true } == { 1, true }) {}", "compound-comparison-root-type"},
		{"field assignments on both sides", "if ({ f1 := 1 } == { f1 := 1 }) {}", "compound-comparison-notation"},
		{"lists of two element types concatenated", "var RI v_i := { 1 }; var RC v_c := { \"a\" }; var RI v_r := v_i & v_c;", "concat-list-root-type"},
		{"record of and set of concatenated", "var RI v_i := { 1 }; var SI v_s := { 1 }; var RI v_r := v_i & v_s;", "concat-list-root-type"},

		{"address of a behaviour type compared with null", "var address v_a := refers(f_a); if (v_a == null) {}", ""},
		{"value lists of one root type", "if ({ 1, 2 } == { 1, 2 }) {}", ""},
		{"lists of one element type concatenated", "var RI v_i := { 1 }; var RI v_r := v_i & v_i;", ""},
		{"integers compared", "var integer v_a := 1, v_b := 2; if (v_a == v_b) {}", ""},
	}
	ruleCodes := []string{"behaviour-type-comparison", "address-behaviour-comparison", "compound-comparison-root-type", "compound-comparison-notation", "concat-list-root-type"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type function F_t();
			type F_t address;
			type record of integer RI;
			type record of charstring RC;
			type set of integer SI;
			function f_a() {}
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
