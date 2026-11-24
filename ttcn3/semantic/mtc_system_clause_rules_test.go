package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A map or unmap with a param clause needs the type of system known: not
// in a function without a system clause (21.1.1, 21.1.2).
func TestMtcSystemClauseRules(t *testing.T) {
	cases := []struct {
		name, header, stmt string
		want               bool
	}{
		{"map param without system clause", "function f() runs on C", "map(self:p, system:p) param (5);", true},
		{"unmap param without system clause", "function f() runs on C", "unmap(self:p, system:p) param (5);", true},
		{"map param with system clause", "function f() runs on C system C", "map(self:p, system:p) param (5);", false},
		{"map param in a test case", "testcase f() runs on C", "map(self:p, system:p) param (5);", false},
		{"map without param", "function f() runs on C", "map(self:p, system:p);", false},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type port P message { inout integer; map param (integer x); unmap param (integer x) }
			type component C { port P p }
			`+c.header+` { `+c.stmt+` }
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		if has := slices.Contains(got, "param-unknown-system-type"); has != c.want {
			t.Errorf("%s: param-unknown-system-type %v, want %v (%v)", c.name, has, c.want, got)
		}
	}
}
