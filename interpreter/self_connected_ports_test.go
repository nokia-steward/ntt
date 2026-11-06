package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// Two ports of one component may be connected to each other (ETSI 9.1,
// figure 6): what one sends the other receives, matched by a template
// like any message, its sender the component itself. On either clock.
func TestSelfConnectedPortsDeliverToEachOther(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p1, p2; timer t := 1.0 }
		testcase tc() runs on C {
			connect(self:p1, self:p2);
			p1.send(5);
			p2.send(7);
			t.start;
			alt {
				[] p2.receive(integer:5) from self { }
				[] t.timeout { setverdict(fail, "p2 got nothing"); stop }
			}
			alt {
				[] p1.check(receive(integer:7)) { p1.receive; setverdict(pass) }
				[] t.timeout { setverdict(fail, "p1 got nothing") }
			}
		}
	}`
	for name, run := range map[string]func(*testing.T, string, string) (runtime.Verdict, string){
		"scheduler":  runSched,
		"real clock": runStrict,
	} {
		t.Run(name, func(t *testing.T) {
			if v, reason := run(t, "M.tc", src); v != runtime.PassVerdict {
				t.Fatalf("verdict = %s (%s), want pass", v, reason)
			}
		})
	}
}
