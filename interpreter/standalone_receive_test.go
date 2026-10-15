package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestStandaloneReceiveWaits: a receiving operation used as a statement is
// an alt with that one alternative (ETSI 20.1). It waits for a match, and
// the active defaults apply.
func TestStandaloneReceiveWaits(t *testing.T) {
	for name, src := range map[string]string{
		// The request/response exchange that ended with verdict none: the
		// MTC's receive and the PTC's returned before the message came.
		"echo": `module M {
			type port P message { inout charstring }
			type component C { port P p }
			function echo() runs on C {
				var charstring s;
				p.receive(charstring:?) -> value s;
				p.send(s);
			}
			testcase tc() runs on C system C {
				var C e := C.create("echo");
				connect(self:p, e:p);
				e.start(echo());
				p.send("ping");
				p.receive("ping");
				setverdict(pass);
			}
		}`,
		// The two-way handshake that ended with verdict none on the
		// virtual clock: the PTC sends first.
		"handshake": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function ptc() runs on C { p.send(1); p.receive(2); setverdict(pass); }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p, c:p);
				c.start(ptc());
				p.receive(1);
				p.send(2);
				c.done;
				setverdict(pass);
			}
		}`,
		// A default whose timer reaches its altstep as a parameter ends the
		// wait of a receive that nothing answers.
		"default": `module M {
			type port P message { inout integer }
			type component C { timer t_guard := 0.5; port P p }
			altstep a(timer t) runs on C { [] t.timeout { setverdict(pass) } }
			testcase tc() runs on C {
				t_guard.start;
				activate(a(t_guard));
				p.receive;
			}
		}`,
		// A check with no receiving operation observes a reply at the head
		// of the queue, not only messages (ETSI 22.4).
		"check": `module M {
			signature S();
			type port PP procedure { inout S }
			type component C { port PP p }
			function server() runs on C { p.getcall; p.reply(S:{}); }
			testcase tc() runs on C system C {
				var C s := C.create;
				connect(self:p, s:p);
				p.call(S:{}, nowait);
				s.start(server());
				p.check;
				timer g := 5.0; g.start;
				alt { [] p.check { setverdict(pass) } [] g.timeout { setverdict(fail, "check saw no reply") } }
				p.getreply;
			}
		}`,
		// A call with no response block does not wait, whether nowait is
		// written or its signature is noblock (ETSI 22.3.1); both are sent.
		"noblock call": `module M {
			signature S(in integer x) noblock;
			type port PP procedure { inout S }
			type component C { port PP p }
			function server() runs on C {
				timer g := 5.0; g.start;
				for (var integer i := 0; i < 2; i := i + 1) {
					alt { [] p.getcall(S:{i}) {} [] g.timeout { setverdict(fail, "no call"); stop; } }
				}
				setverdict(pass);
			}
			testcase tc() runs on C system C {
				var C s := C.create, k := C.create;
				connect(s:p, k:p);
				s.start(server());
				k.start(client());
				timer g := 5.0; g.start;
				alt { [] s.done {} [] g.timeout { setverdict(fail, "the server got no calls") } }
			}
			function client() runs on C { p.call(S:{0}); p.call(S:{1}); }
		}`,
		// any port is this component's own ports: the call the MTC sent sits
		// in the PTC's queue, and is not the MTC's to check.
		"any port": `module M {
			signature S();
			type port PP procedure { inout S }
			type component C { port PP p }
			function server() runs on C { p.getcall; p.reply(S:{}); }
			altstep replied() runs on C { [] any port.getreply { setverdict(pass); stop; } }
			testcase tc() runs on C system C {
				var C s := C.create;
				activate(replied());
				connect(self:p, s:p);
				p.call(S:{}, nowait);
				s.start(server());
				any port.check(from self);
				setverdict(fail, "checked the call the MTC sent");
			}
		}`,
		// @nodefault on the statement is the alt's (22.2.2): the default
		// that would end the wait early is not consulted.
		"@nodefault": `module M {
			type port P message { inout integer }
			type component C { port P p; timer g := 0.1 }
			function late() runs on C { timer t := 0.4; t.start; t.timeout; p.send(2); }
			altstep early() runs on C { [] g.timeout { setverdict(fail, "the default was consulted") } }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p, c:p);
				c.start(late());
				g.start;
				activate(early());
				@nodefault p.receive(integer:2);
				setverdict(pass);
			}
		}`,
		// A default's branch is behaviour: a receive in it waits. The
		// default is taken without repeat, so the alt that invoked it ends.
		"default branch": `module M {
			type port P message { inout integer }
			type component C { port P p; var integer got := 0 }
			function f() runs on C { p.send(1); timer t := 0.3; t.start; t.timeout; p.send(2); p.send(3); }
			altstep a() runs on C { [] p.receive(integer:1) { p.receive(integer:2); got := 2 } }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p, c:p);
				c.start(f());
				activate(a());
				timer g := 3.0; g.start;
				alt { [] p.receive(integer:3) { setverdict(fail, "3 first") } [] g.timeout { setverdict(fail, "timeout") } }
				alt { [] p.receive(integer:3) { if (got == 2) { setverdict(pass) } } [] g.timeout { setverdict(fail, "timeout 2") } }
			}
		}`,
		// any from pa.receive as a statement waits like the alt it is.
		"any from": `module M {
			type port P message { inout integer }
			type component C { port P pa[2]; port P p }
			function sendSeven() runs on C { timer t := 0.3; t.start; t.timeout; p.send(7); }
			testcase tc() runs on C system C {
				var C c0 := C.create, c1 := C.create;
				connect(self:pa[0], c0:p); connect(self:pa[1], c1:p);
				c1.start(sendSeven());
				var integer v := 0;
				any from pa.receive(integer:?) -> value v;
				if (v == 7) { setverdict(pass) } else { setverdict(fail, "v=", v) }
			}
		}`,
		// A timer given to an activated altstep by name (a(t := g)) is the
		// one the wait for the default watches.
		"named timer": `module M {
			type port P message { inout integer }
			type component C { port P p }
			altstep a(timer t) runs on C { [] t.timeout { setverdict(pass) } }
			testcase tc() runs on C system C {
				timer g := 0.3; g.start;
				activate(a(t := g));
				p.receive;
			}
		}`,
		// mtc.stop from a PTC ends the MTC's wait (21.3.3).
		"mtc.stop": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function stopper() runs on C { p.send(1); timer t := 0.3; t.start; t.timeout; setverdict(pass); mtc.stop; }
			testcase tc() runs on C system C {
				var C c := C.create;
				connect(self:p, c:p);
				c.start(stopper());
				p.receive(integer:1);
				p.receive(integer:99);
				setverdict(fail, "the MTC ran on after mtc.stop");
			}
		}`,
		// Stopping and restarting an alive component many times: the
		// restart waits for the stopped behaviour, and misses no finish.
		"restart": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function blocked() runs on C { p.receive(integer:99); setverdict(fail, "the stopped behaviour ran on"); }
			function answer() runs on C { p.send(1); }
			testcase tc() runs on C system C {
				var C c := C.create alive;
				connect(self:p, c:p);
				timer g := 20.0; g.start;
				for (var integer i := 0; i < 60; i := i + 1) {
					c.start(blocked());
					c.stop;
					c.start(answer());
					alt { [] p.receive(integer:1) {} [] g.timeout { setverdict(fail, "no answer at ", i); stop } }
					c.done;
				}
				setverdict(pass);
			}
		}`,
	} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestStartOfARunningComponentIsAnError: an alive component whose behaviour
// runs, not stopped, cannot be started again (ETSI 21.3.2).
func TestStartOfARunningComponentIsAnError(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function waiter() runs on C { p.receive(integer:1); }
		function other() runs on C { }
		testcase tc() runs on C system C {
			var C c := C.create alive;
			connect(self:p, c:p);
			c.start(waiter());
			c.start(other());
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if v != runtime.ErrorVerdict {
			t.Errorf("%s clock: %s (%s), want error", k.name, v, reason)
		}
	}
}

// TestQualifiedHelperRunsAsItsComponent: a PTC body whose communication is
// in a function of another module, called by its qualified name, runs as
// its component and may wait; it used to run inline and could not.
func TestQualifiedHelperRunsAsItsComponent(t *testing.T) {
	h := parse(t, `module H {
		import from M all;
		function helper() runs on C { p.receive(integer:1); setverdict(pass); }
	}`)
	m := parse(t, `module M {
		import from H all;
		type port P message { inout integer }
		type component C { port P p }
		testcase tc() runs on C system C {
			var C c := C.create;
			connect(self:p, c:p);
			c.start(H.helper());
			p.send(1);
			c.done;
		}
	}`)
	for _, k := range clocks {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		opts := k.opts
		opts.Context = ctx
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{m, h}, "M.tc", opts)
		cancel()
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}
