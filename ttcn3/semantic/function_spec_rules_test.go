package semantic

import "testing"

// TestNoRunsOnInvokesRunsOn: behaviour without a runs on clause shall
// not invoke, or activate as default, behaviour with one locally (16.1
// a, 16.2.0 d); starting it on another component is not local (16.1 b).
func TestNoRunsOnInvokesRunsOn(t *testing.T) {
	const code = "function-no-runs-on-invokes-runs-on"
	for _, tt := range []struct {
		name, src string
		reject    bool
	}{
		{"activate from an altstep", `module M {
			type component C { timer T; }
			altstep d() runs on C { [] T.timeout {} }
			altstep a(timer p_t) { [] p_t.timeout { var default v := activate(d()); } }
		}`, true},
		{"activate from a runs on altstep", `module M {
			type component C { timer T; }
			altstep d() runs on C { [] T.timeout {} }
			altstep a() runs on C { [] T.timeout { var default v := activate(d()); } }
		}`, false},
		{"start from a function", `module M {
			type component C {}
			function f() runs on C {}
			function g(C p_c) { p_c.start(f()); }
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
