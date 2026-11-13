package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestIndexOutOfBounds(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		// NegSem_060203_..._013
		{"read past literal", `var RoI v := { 0, 1, 2 }; var integer i := v[3];`, true},
		{"read inside literal", `var RoI v := { 0, 1, 2 }; var integer i := v[2];`, false},

		// 7.1.8.0: the operand of a presence check yields false.
		{"isbound operand", `var RoI v := { 1, 2 }; var boolean b := isbound(v[5]);`, false},
		{"ispresent field after index", `var RoR v := { { f := 1 } }; var boolean b := ispresent(v[3].f);`, false},
		{"nested index operand", `var RoRoI v := { { 1 } }; var boolean b := isbound(v[4][0]);`, false},
		{"index inside index still resolved", `var RoI v := { 1, 2 }; var RoI w := { 0 }; var boolean b := ispresent(v[w[3]]);`, true},
		{"presence check does not cover siblings", `var RoI v := { 1 }; var boolean b := isbound(v[0]) and v[2] == 1;`, true},

		// 5.4.2: out / inout actual parameters expand the value.
		{"out parameter", `var RoI v := { 0 }; f_out(v[2]); var integer i := v[2];`, false},
		{"in parameter still read", `var RoI v := { 0 }; f_in(v[2]);`, true},

		// 6.2.7: ranged dimensions index from the lower bound.
		{"ranged array type", `var A v := { 1, 2, 3, 4 }; var integer i := v[2] + v[5];`, false},
		{"ranged array type below lower bound", `var A v := { 1, 2, 3, 4 }; var integer i := v[1];`, true},
		{"ranged array type past upper bound", `var A v := { 1, 2, 3, 4 }; var integer i := v[6];`, true},
		{"ranged declarator", `var integer v[4 .. 6] := { 40, 50, 60 }; var integer i := v[4] + v[6];`, false},
		{"ranged declarator past upper bound", `var integer v[4 .. 6] := { 40, 50, 60 }; var integer i := v[7];`, true},

		// 15.6.3: transformed form of record of templates.
		{"permutation elements", `var template RoI v := { permutation(0, 1, 3, ?), 2, ? }; var template integer i := v[4];`, false},
		{"past permutation", `var template RoI v := { permutation(0, 1, 3, ?), 2, ? }; var template integer i := v[6];`, true},
		{"permutation with AnyElementsOrNone", `var template RoI v := { permutation(0, 1, 3, *), 2, ? }; var template integer i := v[5];`, false},
		{"fixed length AnyElementsOrNone", `var template RoI v := { 1, * length(2), 5 }; var template integer i := v[3];`, false},
		{"past fixed length AnyElementsOrNone", `var template RoI v := { 1, * length(2), 5 }; var template integer i := v[4];`, true},
		{"ranged length AnyElementsOrNone", `var template RoI v := { 1, * length(1 .. 2), 5 }; var template integer i := v[4];`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := parse(t, `module M {
				type record of integer RoI;
				type record of RoI RoRoI;
				type record R { integer f optional }
				type record of R RoR;
				type integer A[2 .. 5];
				type component C {}
				function f_out(out integer p) { p := 2 }
				function f_in(in integer p) {}
				testcase TC() runs on C { `+tt.body+` }
			}`)
			diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
			if got := containsCode(diags, "index-out-of-bounds"); got != tt.want {
				t.Fatalf("index-out-of-bounds = %v, want %v; got %v", got, tt.want, codes(diags))
			}
		})
	}
}
