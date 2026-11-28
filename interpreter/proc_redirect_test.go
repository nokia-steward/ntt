package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func TestProcedureGetreplyRedirectBindsReturnAndParams(t *testing.T) {
	tree := parse(t, `module ProcRedirectSmoke {
		signature S(out integer p_out) return integer;
		type port P procedure { inout S; }
		type component C { port P p; }

		testcase tc_GetreplyRedirect() runs on C system C {
			var integer v_ret := 0;
			var integer v_param := 0;
			p.reply(S:{ p_out := 4 } value 7);
			alt {
				[] p.getreply(S:?) -> value v_ret { }
				[] p.getreply { setverdict(fail, "unredirected fallback"); }
			}
			p.reply(S:{ p_out := 5 } value 8);
			alt {
				[] p.getreply(S:?) -> param(v_param := p_out) { }
				[] p.getreply { setverdict(fail, "unredirected fallback"); }
			}
			if (v_ret == 7 and v_param == 5) {
				setverdict(pass);
			} else {
				setverdict(fail, "wrong redirect values", v_ret, v_param);
			}
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ProcRedirectSmoke.tc_GetreplyRedirect")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

func TestBlockingProcedureCallRunsDeferredResponder(t *testing.T) {
	tree := parse(t, `module ProcBlockingCallSmoke {
		signature S(out bitstring p_out);
		type port P procedure { inout S; }
		type component C { port P p; }

		function server() runs on C {
			var charstring v_src := "abc";
			var bitstring v_bits := encvalue(v_src);
			p.getcall(S:?);
			p.reply(S:{ p_out := v_bits });
		}

		testcase tc_BlockingCallRedirect() runs on C system C {
			var C v_ptc := C.create("ptc");
			var charstring v_decoded := "";
			connect(self:p, v_ptc:p);
			v_ptc.start(server());
			p.call(S:{ p_out := - }) {
				[] p.getreply(S:?) -> param(v_decoded := @decoded p_out) { }
				[] p.getreply { setverdict(fail, "fallback"); }
			}
			if (v_decoded == "abc") {
				setverdict(pass);
			} else {
				setverdict(fail, "decoded", v_decoded);
			}
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ProcBlockingCallSmoke.tc_BlockingCallRedirect")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
