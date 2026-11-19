package interpreter_test

import "testing"

// any from <port array>.check checks the ports from the lowest index, binds
// the index of the first that succeeds and leaves its message queued (ETSI
// 22.4).
func TestAnyFromCheck_BindsTheIndexAndLeavesTheMessage(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type port MP message { inout integer; }
		type component C { port MP src[3]; port MP pa[3]; timer T := 1.0; }
		testcase tc() runs on C {
			var integer v_idx := -1, v_val := -1;
			connect(mtc:src[1], mtc:pa[1]);
			connect(mtc:src[2], mtc:pa[2]);
			src[2].send(2);
			src[1].send(1);
			T.start;
			alt { [] pa[1].check { } [] T.timeout { setverdict(fail); stop; } }
			alt { [] pa[2].check { } [] T.timeout { setverdict(fail); stop; } }
			alt {
				[] any from pa.check(receive(integer:?) -> value v_val @index value v_idx) { }
				[] T.timeout { setverdict(fail); stop; }
			}
			if (v_idx == 1 and v_val == 1) { pa[1].receive(1); setverdict(pass); }
			else { setverdict(fail, v_idx, v_val); }
		}
	}`)
}
