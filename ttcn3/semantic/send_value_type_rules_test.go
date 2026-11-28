package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// The type of a literal sent on a message port is known (15.4).
func TestSendValueTypeRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"literal of two outgoing subtypes", "p.send(5);", "send-value-type-ambiguous"},

		{"literal with its type named", "p.send(I1: 5);", ""},
		{"literal of one outgoing type", "p.send(\"a\");", ""},
		{"literal on a port of one integer type", "q.send(5);", ""},
	}
	for _, c := range cases {
		tree := parse(t, `module M {
			type integer I1 (0..10);
			type integer I2 (0..100);
			type port P message { inout I1, I2, charstring }
			type port Q message { inout integer; in I1 }
			type component C { port P p; port Q q }
			testcase TC() runs on C {
				`+c.stmt+`
			}
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		has := slices.Contains(got, "send-value-type-ambiguous")
		switch {
		case c.want == "" && has:
			t.Errorf("%s: unexpected send-value-type-ambiguous", c.name)
		case c.want != "" && !has:
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}
