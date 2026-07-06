package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestUnqualifiedGetreplyIsScopedToCallSignature covers ETSI 22.3.1 h:
// an unqualified getreply inside a blocking call(S2,...){ } response
// block treats only S2's reply. Here the server also replies to an
// earlier, unhandled S1 call, so an S1 reply clogs the port queue; the
// in-block getreply must NOT match it, letting the block fall through
// to its timeout branch. Mirrors Sem_220301_CallOperation_019.
func TestUnqualifiedGetreplyIsScopedToCallSignature(t *testing.T) {
	tree := parse(t, `module ProcSigQualReply {
		signature S1() noblock;
		signature S2();
		type port P procedure { inout S1, S2; }
		type component C { port P p; }

		function f_called() runs on C {
			p.getcall(S1:?);
			p.getcall(S2:?);
			p.reply(S1:{});
			p.reply(S2:{});
		}

		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			connect(self:p, v_ptc:p);
			v_ptc.start(f_called());
			p.call(S1:{});
			p.call(S2:{}, 1.0) {
				[] p.getreply { setverdict(fail, "matched clogging S1 reply"); }
				[] p.catch(timeout) { setverdict(pass); }
			}
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ProcSigQualReply.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestUnqualifiedCatchIsScopedToCallSignature is the exception analogue:
// an unqualified catch inside call(S2,...){ } treats only S2's raised
// exception, ignoring an S1 exception left in the queue by an earlier
// unhandled call. Mirrors Sem_220301_CallOperation_020.
func TestUnqualifiedCatchIsScopedToCallSignature(t *testing.T) {
	tree := parse(t, `module ProcSigQualCatch {
		signature S1() noblock exception(integer);
		signature S2() exception(charstring);
		type port P procedure { inout S1, S2; }
		type component C { port P p; }

		function f_called() runs on C {
			p.getcall(S1:?);
			p.getcall(S2:?);
			p.raise(S1, 1);
			p.raise(S2, "exc");
		}

		testcase tc() runs on C system C {
			var C v_ptc := C.create;
			connect(self:p, v_ptc:p);
			v_ptc.start(f_called());
			p.call(S1:{});
			p.call(S2:{}, 1.0) {
				[] p.catch { setverdict(fail, "matched clogging S1 exception"); }
				[] p.catch(timeout) { setverdict(pass); }
			}
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "ProcSigQualCatch.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
