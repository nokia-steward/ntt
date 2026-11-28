package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// Predefined functions and operators take arguments of their parameters'
// root types (annex C, 7.1.8.2, 7.3, 16.1.6).
func TestPredefinedArgTypeRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"any2unistr format", `var universal charstring v := any2unistr(5, 1);`, "predefined-argument-type"},
		{"decvalue of an octetstring", `var octetstring o := '41'O; var integer i; var integer v := decvalue(o, i);`, "predefined-argument-type"},
		{"str2int of an integer", `var integer v := str2int(5);`, "predefined-argument-type"},
		{"not implemented with an integer", `???(42);`, "predefined-argument-type"},
		{"decoding an integer", `var integer i := 5; var R v := i => R;`, "decoded-field-reference-operand"},
		{"encoding format for a bitstring", `var bitstring b := '01'B; var R v := b => (R, "UTF-8");`, "decoded-field-reference-format"},
		{"encoding format of an integer", `var universal charstring u := "x"; var R v := u => (R, 8);`, "decoded-field-reference-format"},
		{"ischosen of a record field", `var R r := { f1 := 1 }; var boolean v := ischosen(r.f1);`, "ischosen-not-an-alternative"},

		{"str2int of a charstring", `var integer v := str2int("5");`, ""},
		{"unichar2int of a charstring", `var integer v := unichar2int("a");`, ""},
		{"decvalue of a bitstring", `var bitstring b := '01'B; var integer i; var integer v := decvalue(b, i);`, ""},
		{"not implemented with a reason", `???("later");`, ""},
		{"decoding an octetstring", `var octetstring o := '41'O; var R v := o => R;`, ""},
		{"encoding format for a universal charstring", `var universal charstring u := "x"; var R v := u => (R, "UTF-8");`, ""},
		{"ischosen of a union alternative", `var U u := { a := 1 }; var boolean v := ischosen(u.a);`, ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C {}
			type record R { integer f1 }
			type union U { integer a, boolean b }
			testcase TC() runs on C { `+c.stmt+` }
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		ruleCodes := []string{"predefined-argument-type", "decoded-field-reference-operand", "decoded-field-reference-format", "ischosen-not-an-alternative"}
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
