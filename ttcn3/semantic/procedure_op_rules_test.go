package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// An exception is raised on a port with the signature in its in or inout
// list, of a type the value pins down (22.3.5); a getreply stores the
// return value into a variable of a compatible type (22.3.4); a range
// boundary is a specific value (B.1.2.5 a).
func TestProcedureOpRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"raise on a port with the signature in its out list", "p_out.raise(S, R1:{ f := 1 });", "port-op-type-not-allowed"},
		{"exception value of two possible types", "p.raise(S, { f := 1 });", "raise-exception-ambiguous"},
		{"integer return value into a charstring", "var charstring v_c; p.getreply(S:{} value 1) -> value v_c;", "value-redirect-type"},
		{"range up to a template", "var template integer v_t := (0 .. mw_any);", "range-bound-template"},

		{"raise on a port with the signature in its inout list", "p.raise(S, R1:{ f := 1 });", ""},
		{"exception value of one possible type", "p.raise(S, { g := 1 });", ""},
		{"integer return value into an integer", "var integer v_i; p.getreply(S:{}) -> value v_i;", ""},
		{"range up to a constant", "var template integer v_t := (0 .. c_max);", ""},
	}
	ruleCodes := []string{"port-op-type-not-allowed", "raise-exception-ambiguous", "value-redirect-type", "range-bound-template"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type record R1 { integer f }
			type record R2 { integer f }
			type record R3 { integer g }
			signature S() return integer exception(R1, R2, R3);
			type port P procedure { inout S }
			type port PO procedure { out S }
			type component C { port P p; port PO p_out }
			template integer mw_any := ?;
			const integer c_max := 10;
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
