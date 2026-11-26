package semantic

import (
	"slices"
	"testing"

	"github.com/nokia/ntt/ttcn3"
)

// A started function runs on a component (16.1.0 b); a @control altstep
// is invoked in control behaviour only (16.2.0).
func TestInvocationPlaceRules(t *testing.T) {
	cases := []struct {
		name, stmt, want string
	}{
		{"function without runs on started", "var C v_ptc := C.create; v_ptc.start(f_free());", "start-without-runs-on"},
		{"control altstep invoked in a testcase", "a_ctrl();", "control-altstep-in-test-behaviour"},
		{"control altstep activated in a testcase", "var default v_d := activate(a_ctrl());", "control-altstep-in-test-behaviour"},

		{"function with runs on started", "var C v_ptc := C.create; v_ptc.start(f_ptc());", ""},
		{"timer started with a computed duration", "timer T; T.start(f_dur());", ""},
		{"altstep without @control", "a_step();", ""},
	}
	ruleCodes := []string{"start-without-runs-on", "control-altstep-in-test-behaviour"}
	for _, c := range cases {
		tree := parse(t, `module M {
			type component C { timer T_g := 0.1 }
			function f_free() {}
			function f_ptc() runs on C {}
			function f_dur() return float { return 1.0; }
			altstep @control a_ctrl() { [] any timer.timeout {} }
			altstep a_step() { [] any timer.timeout {} }
			function @control f_ctl() { a_ctrl(); }
			testcase TC() runs on C {
				`+c.stmt+`
			}
			control { a_ctrl(); }
		}`)
		got := codes(NewAnalyzer(&ttcn3.DB{}).Analyze(tree))
		var hit []string
		for _, code := range got {
			if slices.Contains(ruleCodes, code) {
				hit = append(hit, code)
			}
		}
		switch {
		case c.want == "" && len(hit) > 0:
			t.Errorf("%s: unexpected %v", c.name, hit)
		case c.want != "" && !slices.Contains(got, c.want):
			t.Errorf("%s: expected %s, got %v", c.name, c.want, got)
		}
	}
}
