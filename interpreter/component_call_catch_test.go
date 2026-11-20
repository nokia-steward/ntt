package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// A component call whose behaviour is stopped by another component runs
// its catch(stop) block, and leaves an inout argument as it was (ETSI
// 21.3.10), also when the stop comes during the behaviour's last
// statement.
func TestComponentCallCatchStop(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type component C {}
		function f(inout integer io) runs on C { io := 99; timer x := 5.0; x.start; x.timeout; }
		function k(C v) runs on C { timer y := 0.3; y.start; y.timeout; v.stop; }
		testcase tc() runs on C system C {
			var C p := C.create, q := C.create;
			var boolean caught := false;
			var integer io := 3;
			q.start(k(p));
			p.call(f(io)) catch(stop) { caught := true }
			if (caught and io == 3) { setverdict(pass) } else { setverdict(fail, caught, io) }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
