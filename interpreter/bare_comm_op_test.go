package interpreter_test

import "testing"

// A receiving operation without a parameter list takes a redirect and a
// from clause as one with a template does: `p.trigger -> sender v`,
// `p.receive from c` (ETSI 22.2.2, 22.2.3).
func TestBareReceivingOperationWithClauses(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type port P message { inout integer }
		type component C { port P p; port P q; timer T := 1.0; }
		testcase tc() runs on C {
			var C v_sender;
			var integer v := -1;
			connect(mtc:p, mtc:q);
			q.send(1); q.send(2); q.send(3);
			T.start;
			alt { [] p.trigger -> sender v_sender { } [] T.timeout { setverdict(fail, "trigger"); stop; } }
			alt { [] p.receive from v_sender { } [] T.timeout { setverdict(fail, "from"); stop; } }
			alt { [] p.receive(integer:?) -> value v { } [] T.timeout { setverdict(fail, "value"); stop; } }
			if (v == 3 and v_sender == mtc) { setverdict(pass); } else { setverdict(fail, v); }
		}
	}`)
}
