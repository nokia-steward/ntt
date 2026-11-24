package semantic

import (
	"slices"
	"strings"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// The index of `any from` over a component or timer array is stored in a
// variable that can hold it (21.3.5 to 21.3.8, 23.5, 23.6), and a timer
// array's any from takes an array.
func TestIndexRedirectArrayRules(t *testing.T) {
	cases := []struct {
		name, decls, stmt, want string
	}{
		{"component index out of range", "var C a[3]; var I0To1 v;", "var boolean b := any from a.alive -> @index value v;", "index-redirect-type-range"},
		{"component array too long", "var C a[2][2]; var integer v[3];", "var boolean b := any from a.running -> @index value v;", "index-redirect-length-mismatch"},
		{"component element out of range", "var C a[3][2]; var I0To1 v[2];", "any from a.done -> @index value v;", "index-redirect-type-range"},
		{"component 1-D into an array", "var C a[3]; var integer v[1];", "any from a.killed -> @index value v;", "index-redirect-array-shape"},
		{"component 2-D into a scalar", "var C a[2][2]; var integer v;", "any from a.done -> @index value v;", "index-redirect-array-shape"},
		{"timer index out of range", "timer a[5]; var I0To2 v;", "any from a.timeout -> @index value v;", "index-redirect-type-range"},
		{"timer array too long", "timer a[2][2]; var integer v[3];", "var boolean b := any from a.running -> @index value v;", "index-redirect-length-mismatch"},
		{"timer element out of range", "timer a[2][5]; var I0To1 v[2];", "var boolean b := any from a.running -> @index value v;", "index-redirect-type-range"},
		{"timer not an array", "timer a := 1.0;", "any from a.timeout;", "any-from-timer-not-array"},
		{"timer not an array, running", "timer a := 1.0;", "var boolean b := any from a.running;", "any-from-timer-not-array"},

		{"component index in range", "var C a[2]; var I0To1 v;", "var boolean b := any from a.alive -> @index value v;", ""},
		{"component 2-D into an integer array", "var C a[2][3]; var integer v[2];", "any from a.done -> @index value v;", ""},
		{"component 2-D into a record of integer", "var C a[2][3]; var Ints v;", "any from a.done -> @index value v;", ""},
		{"timer element in range", "timer a[2][3]; var I0To2 v[2];", "any from a.timeout -> @index value v;", ""},
		{"timer ranged dimension", "timer a[1..2]; var I0To2 v;", "any from a.timeout -> @index value v;", ""},
		{"timer array without index", "timer a[2];", "any from a.timeout;", ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C {}
			type integer I0To1 (0..1);
			type integer I0To2 (0..2);
			type record of integer Ints;
			testcase TC() runs on C {
				`+c.decls+`
				`+c.stmt+`
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var index []string
		for _, code := range got {
			if strings.HasPrefix(code, "index-redirect-") || code == "any-from-timer-not-array" {
				index = append(index, code)
			}
		}
		switch {
		case c.want == "" && len(index) > 0:
			t.Errorf("%s: unexpected %v", c.name, index)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}
