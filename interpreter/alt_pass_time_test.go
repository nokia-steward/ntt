package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// Every alternative of an alt pass reads timers at one instant (ETSI ES
// 201 873-1 20.1): both timers expire while the second alternative's
// guard computes, after the first alternative found its timer running.
// The second's timeout does not see its timer expire; the next pass takes
// the first. On the virtual clock no time passes in a pass anyway.
func TestAltPassReadsTimersAtOneInstant(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type component C {}
		function busy() return boolean {
			var integer i, s := 0;
			for (i := 0; i < 20000; i := i + 1) { s := s + 1; }
			return s > 0;
		}
		testcase tc() runs on C {
			timer t1 := 0.01, t2 := 0.01;
			t1.start;
			t2.start;
			alt {
				[] t1.timeout { setverdict(pass) }
				[busy()] t2.timeout { setverdict(fail, "the second alternative was taken") }
			}
		}
	}`, runtime.PassVerdict)
}

// So do the defaults an alt invokes, newest first: the timer expires while
// the older default's guard computes, after the newer one found it
// running. The older does not see it expire; the next pass gives it to
// the newer.
func TestAltDefaultsReadTimersAtOneInstant(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type component C { timer T; var charstring v_log := "" }
		function busy() return boolean {
			var integer i, s := 0;
			for (i := 0; i < 5000; i := i + 1) { s := s + 1; }
			return s > 0;
		}
		altstep a(charstring s) runs on C { [] T.timeout { v_log := v_log & s } }
		altstep slow() runs on C { [busy()] T.timeout { v_log := v_log & "slow" } }
		testcase tc() runs on C {
			var default d1 := activate(slow()), d2 := activate(a("2"));
			timer g := 10.0;
			T.start(0.01); g.start;
			alt { [] g.timeout { v_log := v_log & "g" } }
			if (v_log == "2") { setverdict(pass) } else { setverdict(fail, "log " & v_log) }
		}
	}`, runtime.PassVerdict)
}
