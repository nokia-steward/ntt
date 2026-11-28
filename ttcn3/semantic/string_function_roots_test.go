package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// substr, replace and regexp return a value of the root type of inpar
// (C.4.1, C.4.2, C.4.3).
func TestStringFunctionResultRoots(t *testing.T) {
	cases := []struct {
		name, decl string
		want       bool
	}{
		{"replace of a bitstring into an octetstring", `var octetstring v := replace('0101'B, 0, 1, '1'B);`, true},
		{"substr of a charstring into an integer", `var integer v := substr("abc", 0, 1);`, true},
		{"regexp of a charstring into a bitstring", `var bitstring v := regexp("abc", charstring:"(a)?+", 0);`, true},
		{"replace of a bitstring into a bitstring", `var bitstring v := replace('0101'B, 0, 1, '1'B);`, false},
		{"substr of a charstring into a universal charstring", `var universal charstring v := substr("abc", 0, 1);`, false},
		{"regexp of a charstring into a charstring", `var charstring v := regexp("abc", charstring:"(a)?+", 0);`, false},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C {}
			testcase TC() runs on C { `+c.decl+` }
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		if has := slices.Contains(got, "type-incompatible-value"); has != c.want {
			t.Errorf("%s: type-incompatible-value %v, want %v (%v)", c.name, has, c.want, got)
		}
	}
}
