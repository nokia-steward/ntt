package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// A return in an altstep leaves the altstep (19.10): taken as an
// alternative, the alternative's own block then runs and the alt is
// left; called, or activated as a default, the behaviour goes on after
// it. The testcase around it goes on in every case.
func TestAltstepReturn(t *testing.T) {
	for name, use := range map[string]string{
		"an alternative": `alt { [] a() { order := order & "B" } [] g.timeout { order := order & "T" } }
			if (order != "rB") { setverdict(fail, order) }`,
		"an alternative in a function": `f();
			if (order != "rBF") { setverdict(fail, order) }`,
		"called": `a();
			if (order != "r") { setverdict(fail, order) }`,
		"a default": `var default d := activate(a());
			alt { [] p2.receive(integer:2) { order := order & "2" } [] g.timeout { order := order & "T" } }
			deactivate(d);
			if (order != "r") { setverdict(fail, order) }`,
	} {
		v, reason := runSched(t, "M.tc", `module M {
			type port P message { inout integer }
			type component C { port P p1, p2; var charstring order := ""; timer g := 1.0 }
			altstep a() runs on C {
				[] p2.receive(integer:1) { order := order & "r"; return; order := order & "X" }
			}
			function f() runs on C {
				alt { [] a() { order := order & "B" } [] g.timeout { order := order & "T" } }
				order := order & "F";
			}
			testcase tc() runs on C {
				connect(self:p1, self:p2);
				p1.send(1);
				g.start;
				`+use+`
				setverdict(pass);
			}
		}`)
		if v != runtime.PassVerdict {
			t.Errorf("%s: verdict = %s (%s), want pass", name, v, reason)
		}
	}
}
