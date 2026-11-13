package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestUnionAltNotChosen(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"read other alternative", `var U v := { a := 1 }; var integer i; i := v.b;`, true},
		{"read chosen alternative", `var U v := { a := 1 }; var integer i; i := v.a;`, false},

		// 5.4.2: an actual out / inout parameter follows the
		// left hand side rules and chooses the alternative.
		{"out parameter chooses", `var U v := { a := 1 }; f_out(v.b); var integer i; i := v.b;`, false},
		{"inout parameter chooses", `var U v := { a := 1 }; f_inout(v.b); var integer i; i := v.b;`, false},
		{"in parameter is a read", `var U v := { a := 1 }; f_in(v.b);`, true},
		{"out parameter chose another", `var U v := { a := 1 }; f_out(v.b); var integer i; i := v.a;`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := parse(t, `module M {
				type union U { integer a, integer b }
				type component C {}
				function f_out(out integer p) { p := 2 }
				function f_inout(inout integer p) { p := 2 }
				function f_in(in integer p) {}
				testcase TC() runs on C { `+tt.body+` }
			}`)
			diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
			if got := containsCode(diags, "union-alt-not-chosen"); got != tt.want {
				t.Fatalf("union-alt-not-chosen = %v, want %v; got %v", got, tt.want, codes(diags))
			}
		})
	}
}
