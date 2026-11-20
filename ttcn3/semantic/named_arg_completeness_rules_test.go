package semantic

import "testing"

// TestNamedArgMissingMandatory: in assignment notation every formal
// parameter without a default shall be assigned, except an out one, for
// which "-" is taken (5.4.2, 21.3.2).
func TestNamedArgMissingMandatory(t *testing.T) {
	const code = "named-arg-missing-mandatory"
	for _, tt := range []struct {
		name, src string
		reject    bool
	}{
		{"in parameter omitted", `module M {
			function f(in integer p_a, in integer p_b) {}
			function g() { f(p_b := 1); }
		}`, true},
		{"inout parameter omitted", `module M {
			function f(in integer p_a, inout integer p_b) {}
			function g() { f(p_a := 1); }
		}`, true},
		{"out parameter omitted", `module M {
			function f(in integer p_i, out integer p_o) return integer { p_o := p_i; return p_o; }
			function g() { log(f(p_i := 1)); }
		}`, false},
		{"out parameter omitted in a start", `module M {
			type component C {}
			function f(out integer p_out, in integer p_in) runs on C {}
			testcase tc() runs on C { var C v := C.create; v.start(f(p_in := 5)); }
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
