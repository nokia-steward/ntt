package semantic

import "testing"

// TestConstReassign: an assignment is a reassignment of a constant only
// when its target resolves to the constant (clause 10).
func TestConstReassign(t *testing.T) {
	const code = "const-reassign"
	for _, tt := range []struct {
		name, src string
		reject    bool
	}{
		{"assignment to a constant", `module M {
			const integer c := 1;
			function f() { c := 2; }
		}`, true},
		{"assignment to a local constant", `module M {
			function f() { const integer c := 1; c := 2; }
		}`, true},
		{"field named like the constant (5.5)", `module M {
			type record R { integer c }
			const R c := { c := 1 };
		}`, false},
		{"set field named like another constant (6.2.2)", `module M {
			const integer field1 := 5;
			type set S { integer field1 }
			function f() { var S v := { field1 := 1 }; }
		}`, false},
		{"variable named like a constant of another function", `module M {
			function f() { const integer x := 1; log(x); }
			function g() { var integer x; x := 2; }
		}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCode(analyzeCodes(t, tt.src), code)
			if got != tt.reject {
				t.Errorf("%s reported: %v, want %v", code, got, tt.reject)
			}
		})
	}
}

// TestConstInitNonDeterministic: constants are runtime constants; only
// those used in type and array definitions shall not call rnd (clause
// 10 b).
func TestConstInitNonDeterministic(t *testing.T) {
	const code = "const-init-non-deterministic"
	for _, tt := range []struct {
		name, src string
		reject    bool
	}{
		{"constant used in a subtype", `module M {
			const float c := rnd(3.14);
			type float F (c);
		}`, true},
		{"constant used in an array definition", `module M {
			function f() return integer { return float2int(rnd() * 10.0); }
			const integer n := f();
			function g() { var integer a[n]; }
		}`, true},
		{"constant used through another constant", `module M {
			const float c := rnd();
			const float d := c;
			type float F (d);
		}`, true},
		{"constant used in initial values (6.2.10.1)", `module M {
			const float c := rnd();
			type component C { var float vc := c; const float cc := c; timer t := c + 1.0; }
		}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCode(analyzeCodes(t, tt.src), code)
			if got != tt.reject {
				t.Errorf("%s reported: %v, want %v", code, got, tt.reject)
			}
		})
	}
}
