package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/runtime"
)

// TestBehaviourErrorIsTheComponentsVerdict: a dynamic error in a PTC's
// behaviour ends that component with verdict error, and the testcase with
// it. It ended with none, so the testcase passed on the MTC's verdict.
func TestBehaviourErrorIsTheComponentsVerdict(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type component C {}
		function f() runs on C { var integer x := undefinedThing; setverdict(pass) }
		testcase tc() runs on C {
			var C c := C.create;
			c.start(f());
			c.done;
			setverdict(pass)
		}
	}`, runtime.ErrorVerdict)
}

// TestBehaviourErrorOnAliveComponent: the same for a component created
// alive, whose behaviour the starter reads back through done's redirect.
func TestBehaviourErrorOnAliveComponent(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type component C {}
		function f() runs on C { setverdict(pass); var integer x := undefinedThing }
		testcase tc() runs on C {
			var C c := C.create alive;
			var verdicttype v;
			c.start(f());
			c.done -> value v;
			if (v == error) { setverdict(pass) } else { setverdict(fail) }
		}
	}`, runtime.ErrorVerdict)
}

// TestStoppedBehaviourIsNoError: a behaviour stopped by its starter while
// waiting is not a behaviour that failed.
func TestStoppedBehaviourIsNoError(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type port P message { inout integer }
		type component C { port P p; timer t := 1.0 }
		function f() runs on C { setverdict(pass); p.receive(integer:?) }
		testcase tc() runs on C {
			var C c := C.create;
			c.start(f());
			t.start; t.timeout;
			c.stop;
			setverdict(pass)
		}
	}`, runtime.PassVerdict)
}

// TestRestartOfKilledComponentIsAnError: a non-alive component is killed
// when its behaviour ends or is stopped, and any component when killed; a
// start after that is an error (ETSI 21.3.2). It started the new
// behaviour.
func TestRestartOfKilledComponentIsAnError(t *testing.T) {
	for name, end := range map[string]string{
		"ended":   "c.done;",
		"stopped": "c.stop;",
		"killed":  "c.kill;",
	} {
		t.Run(name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, `module M {
				type component C {}
				function f() runs on C { setverdict(pass) }
				testcase tc() runs on C {
					var C c := C.create;
					c.start(f());
					`+end+`
					c.start(f());
					setverdict(pass)
				}
			}`, runtime.ErrorVerdict)
		})
	}
}

// TestRestartOfStoppedAliveComponent: an alive component, stopped, can
// be started again (ETSI 21.3.3); killed, it cannot.
func TestRestartOfStoppedAliveComponent(t *testing.T) {
	src := `module M {
		type component C {}
		function f() runs on C { setverdict(pass) }
		testcase tc() runs on C {
			var C c := C.create alive;
			c.start(f());
			c.done;
			c.stop;
			c.start(f());
			c.done;
			END
			setverdict(pass)
		}
	}`
	assertSameVerdictBothClocks(t, strings.Replace(src, "END", "", 1), runtime.PassVerdict)
	assertSameVerdictBothClocks(t, strings.Replace(src, "END", "c.kill; c.start(f());", 1), runtime.ErrorVerdict)
}

// TestAliveComponentCalledAgain: an alive component stays alive after a
// call, to be called or started again (ETSI 21.3.10, whose example calls
// one twice); a non-alive one does not.
func TestAliveComponentCalledAgain(t *testing.T) {
	src := `module M {
		type component C {}
		function f() runs on C { setverdict(pass) }
		testcase tc() runs on C {
			var C c := C.create MOD;
			c.call(f());
			c.call(f());
			c.start(f());
			c.done;
			if (c.alive) { setverdict(pass) } else { setverdict(fail) }
		}
	}`
	assertSameVerdictBothClocks(t, strings.Replace(src, "MOD", "alive", 1), runtime.PassVerdict)
	assertSameVerdictBothClocks(t, strings.Replace(src, "MOD", "", 1), runtime.ErrorVerdict)
}

// TestCalledComponentIsKilledByAll: a component that ran a behaviour by
// call is one of those `all component.kill` kills (ETSI 21.3.4), as one
// started is.
func TestCalledComponentIsKilledByAll(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type component C {}
		function f() runs on C { setverdict(pass) }
		testcase tc() runs on C {
			var C c := C.create alive;
			c.call(f());
			all component.kill;
			if (c.alive) { setverdict(fail, "alive after all component.kill") }
			c.call(f());
		}
	}`, runtime.ErrorVerdict)
}

// TestBlockingCallNeedsItsBlock: a call of a blocking signature waits for
// the reply in its response and exception handling part; one with neither
// that part nor nowait is an error (ETSI 22.3.1 j). It was sent, and the
// caller went on as if nowait.
func TestBlockingCallNeedsItsBlock(t *testing.T) {
	src := `module M {
		signature S() return integer;
		signature N(in integer x) noblock;
		type port P procedure { inout S, N }
		type component C { port P p }
		template S s_call := {}
		function viaParam(template N s_call) runs on C { p.call(s_call) }
		function srv() runs on C {
			alt {
				[] p.getcall(S:{}) { p.reply(S:{} value 1) }
				[] p.getcall(N:?) {}
			}
		}
		testcase tc() runs on C {
			var C c := C.create;
			connect(mtc:p, c:p);
			c.start(srv());
			CALL
			setverdict(pass)
		}
	}`
	for _, tc := range []struct {
		call string
		want runtime.Verdict
	}{
		{"p.call(S:{});", runtime.ErrorVerdict},
		{"p.call(s_call);", runtime.ErrorVerdict},
		{"p.call(S:{}) to c;", runtime.ErrorVerdict},
		{"p.call(S:{}, nowait); p.getreply(S:{});", runtime.PassVerdict},
		{"p.call(S:{}) { [] p.getreply(S:{}) {} }", runtime.PassVerdict},
		{"p.call(N:{1});", runtime.PassVerdict},
		// The parameter, not the like-named blocking template outside.
		{"viaParam(N:{1});", runtime.PassVerdict},
	} {
		t.Run(tc.call, func(t *testing.T) {
			assertSameVerdictBothClocks(t, strings.Replace(src, "CALL", tc.call, 1), tc.want)
		})
	}
}
