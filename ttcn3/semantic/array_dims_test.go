package semantic

import (
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

func TestArrayDimConstness(t *testing.T) {
	tests := []struct {
		name  string
		decls string
		want  bool
	}{
		// Clause 10 b): literals, operators, predefined functions
		// except rnd, and other such constants.
		{"predefined function and constants", `
			const integer c_base := 2;
			const integer c_len := lengthof("abc") + c_base * 2 - 3;`, false},
		{"constant used twice", `
			const integer c_base := 2;
			const integer c_len := c_base * c_base;`, false},
		// NegSem_060207_arrays_011 / _012
		{"user function", `
			function f() return integer { return 2 }
			const integer c_len := f();`, true},
		{"rnd", `
			const integer c_len := float2int(rnd(1.0) * 4.0) + 1;`, true},
		{"constant of a user function", `
			function f() return integer { return 2 }
			const integer c_base := f();
			const integer c_len := c_base + 1;`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree := parse(t, `module M {
				`+tt.decls+`
				type integer A[c_len];
			}`)
			diags := NewAnalyzer(&ttcn3.DB{}).Analyze(tree)
			if got := containsCode(diags, "array-dim-non-const"); got != tt.want {
				t.Fatalf("array-dim-non-const = %v, want %v; got %v", got, tt.want, codes(diags))
			}
		})
	}
}
