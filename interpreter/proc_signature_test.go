package interpreter_test

import (
	"strings"
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

// TestGetreplyTakesOnlyItsSignature: a getreply, getcall or catch
// qualified by a signature takes nothing of another (ETSI 22.3.4): with
// the reply to S1 at the head of the queue, getreply(S2:{} value ?) took
// it, and its value.
func TestGetreplyTakesOnlyItsSignature(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		signature S1() return integer;
		signature S2() return integer;
		type port P procedure { inout S1, S2 }
		type component C { port P p }
		template S2 s_two := {}
		function srv() runs on C {
			p.getcall(S1:{}); p.reply(S1:{} value 1);
			p.getcall(S2:{}); p.reply(S2:{} value 2);
		}
		testcase tc() runs on C {
			var C c := C.create; connect(mtc:p, c:p); c.start(srv());
			var integer r := 0;
			timer t := 1.0;
			p.call(S1:{}, nowait);
			p.call(S2:{}, nowait);
			t.start;
			alt {
				[] p.getreply(S2:{} value ?) -> value r { setverdict(fail, "S2 took ", r) }
				[] p.getreply(s_two value ?) -> value r { setverdict(fail, "s_two took ", r) }
				[] p.getreply(S1:{} value 1) { setverdict(pass) }
				[] t.timeout { setverdict(fail, "nothing") }
			}
		}
	}`, runtime.PassVerdict)
}

// TestGetreplyValueAndParamRedirect: `-> value v param (x)` binds both,
// in a call's response block and in a getreply of its own. The parser
// took `v param (x)` for one value target, and neither was bound.
func TestGetreplyValueAndParamRedirect(t *testing.T) {
	src := `module M {
		signature S(out integer x) return integer;
		type port P procedure { inout S }
		type component C { port P p }
		function srv() runs on C { p.getcall(S:{x := -}); p.reply(S:{x := 7} value 5) }
		testcase tc() runs on C {
			var C c := C.create; connect(mtc:p, c:p); c.start(srv());
			var integer v, x;
			GET
			if (v == 5 and x == 7) { setverdict(pass) } else { setverdict(fail, v, x) }
		}
	}`
	for _, get := range []string{
		`p.call(S:{x := -}) { [] p.getreply(S:{x := ?} value ?) -> value v param (x) {} }`,
		`p.call(S:{x := -}, nowait); p.getreply(S:{x := ?} value ?) -> value v param (x);`,
	} {
		assertSameVerdictBothClocks(t, strings.Replace(src, "GET", get, 1), runtime.PassVerdict)
	}
}

// TestCallThroughTemplateParameter: a call made with a template passed as
// a parameter records the parameter's name, not the signature's; the
// unqualified getreply of its block (22.3.1 h) refused the reply for it.
// And a signature named with its module, M.S, is S.
func TestCallThroughTemplateParameter(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		signature S1() return integer;
		signature S2() return integer;
		type port P procedure { inout S1, S2 }
		type component C { port P p }
		function srv() runs on C { p.getcall(S1:{}); p.reply(S1:{} value 1) }
		function ask(template S1 tp) runs on C {
			p.call(tp) {
				[] p.getreply(M.S2:{} value ?) { setverdict(fail, "M.S2 took S1's reply") }
				[] p.getreply { setverdict(pass) }
			}
		}
		testcase tc() runs on C {
			var C c := C.create; connect(mtc:p, c:p); c.start(srv());
			ask(S1:{});
		}
	}`, runtime.PassVerdict)
}

// TestTemplateParameterHasItsSignature: a template passed as a formal
// parameter is of the parameter's signature: getreply(tp value ?) with tp
// of S2 takes no reply to S1.
func TestTemplateParameterHasItsSignature(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		signature S1() return integer;
		signature S2() return integer;
		type port P procedure { inout S1, S2 }
		type component C { port P p }
		function srv() runs on C { p.getcall(S1:{}); p.reply(S1:{} value 1) }
		function get(template S2 tp) runs on C {
			timer t := 1.0; t.start;
			alt {
				[] p.getreply(tp value ?) { setverdict(fail, "S2 template took S1's reply") }
				[] p.getreply(S1:{} value 1) { setverdict(pass) }
				[] t.timeout { setverdict(fail, "nothing") }
			}
		}
		testcase tc() runs on C {
			var C c := C.create; connect(mtc:p, c:p); c.start(srv());
			p.call(S1:{}, nowait);
			get(S2:{});
		}
	}`, runtime.PassVerdict)
}

// TestTestcaseTemplateParameterHasItsSignature: a testcase's template
// parameter is of its declared signature, as a function's is, so the
// unqualified getreply of a call made with it (22.3.1 h) leaves a reply
// to another signature alone.
func TestTestcaseTemplateParameterHasItsSignature(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		signature S1() return integer;
		signature S2() return integer;
		type port P procedure { inout S1, S2 }
		type component C { port P p }
		function srv() runs on C { p.getcall(S2:{}); p.reply(S2:{} value 2) }
		testcase tc(template S1 tq := S1:{}) runs on C {
			var C c := C.create; connect(mtc:p, c:p); c.start(srv());
			p.call(S2:{}, nowait);
			timer w := 0.5; w.start; w.timeout;
			p.call(tq, 1.0) {
				[] p.getreply { setverdict(fail, "took the reply to S2") }
				[] p.catch(timeout) { setverdict(pass) }
			}
		}
	}`, runtime.PassVerdict)
}
