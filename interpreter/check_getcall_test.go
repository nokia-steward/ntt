package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// check takes getcall as it takes the other receiving operations (ETSI
// 22.4): it matches the call at the head of the queue without removing
// it, written with a template or bare.
func TestCheckGetcall(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		signature S(in integer x);
		type port PP procedure { inout S }
		type component C { port PP q1, q2; timer t := 1.0 }
		testcase tc() runs on C {
			connect(self:q1, self:q2);
			q1.call(S:{1}, nowait);
			t.start;
			alt {
				[] q2.check(getcall(S:{1})) { }
				[] t.timeout { setverdict(fail, "check(getcall(S:{1})) did not match"); stop }
			}
			alt {
				[] q2.check(getcall) { }
				[] t.timeout { setverdict(fail, "check(getcall) did not match"); stop }
			}
			alt {
				[] q2.getcall(S:{1}) { setverdict(pass) }
				[] t.timeout { setverdict(fail, "check removed the call") }
			}
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
