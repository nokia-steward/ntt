package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// portControlModule wraps a testcase body in a module whose MTC has two
// connected ports, p1 sending to p2, and helpers to assert what p2's
// receiving operations see.
func portControlModule(body string) string {
	return `module M {
		type port P message { inout integer }
		type component C { port P p1, p2 }
		function want(boolean b, charstring what) {
			if (not b) { setverdict(fail, what); stop }
		}
		function f_sent(integer i) runs on C {
			p1.send(i);
			timer t := 1.0; t.start;
			alt { [] p2.check(receive) { t.stop } [] t.timeout { setverdict(fail, "never arrived"); stop } }
		}
		function f_sees(integer i) runs on C return boolean {
			alt { [] p2.check(receive(i)) { return true } [else] { return false } }
		}
		function f_takes(integer i) runs on C return boolean {
			alt { [] p2.receive(i) { return true } [else] { return false } }
		}
		testcase tc() runs on C {
			connect(self:p1, self:p2);
			` + body + `
			setverdict(pass);
		}
	}`
}

// The port controlling operations act on the queue as ETSI 22.5.2-22.5.4
// and figures F.7 and F.8 define: start clears it, stop hides it from the
// receiving operations, and halt hides only what arrives after it.
func TestPortControl(t *testing.T) {
	tests := []struct{ name, body string }{
		{"start clears the queue of a started port", `
			f_sent(1);
			p2.start;
			want(not f_sees(1), "start kept the queue");
			f_sent(2);
			want(f_takes(2), "a restarted port receives");`},
		{"stop hides the queue, and start then clears it", `
			f_sent(1);
			p2.stop;
			want(not f_sees(1), "receive matched at a stopped port");
			want(not f_takes(1), "receive took from a stopped port");
			p2.start;
			want(not f_sees(1), "start kept the stopped port's queue");`},
		{"halt keeps what was queued before it", `
			f_sent(1);
			f_sent(2);
			p2.halt;
			p1.send(3);
			want(f_takes(1) and f_takes(2), "halt hid the entries ahead of the marker");
			want(not f_sees(3), "an entry after the halt marker matched");`},
		{"a halted port with nothing ahead of the marker is like a stopped one", `
			p2.halt;
			p1.send(1);
			want(not f_sees(1), "an entry after the halt marker matched");`},
		{"halting a halted port leaves the marker", `
			f_sent(1);
			p2.halt;
			p1.send(2);
			p2.halt;
			want(f_takes(1), "the entry ahead of the marker");
			want(not f_sees(2), "the second halt moved the marker");`},
		{"start on a halted port clears the queue and removes the marker", `
			f_sent(1);
			p2.halt;
			p1.send(2);
			p2.start;
			want(not f_sees(1) and not f_sees(2), "start kept the halted port's queue");
			f_sent(3);
			want(f_takes(3), "the marker outlived start");`},
		{"stop on a halted port hides what was ahead of the marker", `
			f_sent(1);
			p2.halt;
			p2.stop;
			want(not f_sees(1), "stop left the halt marker");`},
		{"halt on a stopped port puts the marker at the top", `
			f_sent(1);
			p2.stop;
			p2.halt;
			want(not f_sees(1), "halt of a stopped port showed its queue");
			want(p2.checkstate("Halted"), "the port is halted");`},
		{"clear on a halted port puts the marker at the top", `
			f_sent(1);
			p2.halt;
			p2.clear;
			p1.send(2);
			want(not f_sees(2), "clear removed the halt marker");`},
		{"any port sees only what each port shows", `
			f_sent(1);
			p2.stop;
			alt { [] any port.receive { setverdict(fail, "any port took from a stopped port"); stop } [else] {} }`},
		{"sending from a started port to a stopped one is no error", `
			p2.stop;
			p1.send(1);`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSameVerdictBothClocks(t, portControlModule(tt.body), runtime.PassVerdict)
		})
	}
}

// Sending operations on a stopped or halted port are errors (ETSI 22.5.3,
// 22.5.4, F.3.3), and start allows them again.
func TestPortControlSending(t *testing.T) {
	src := func(control, op string) string {
		return `module M {
			type port P message { inout integer }
			signature S(in integer x) exception (integer);
			type port Q procedure { inout S }
			type component C { port P p1, p2; port Q q1, q2 }
			testcase tc() runs on C {
				connect(self:p1, self:p2);
				connect(self:q1, self:q2);
				` + control + `
				` + op + `
				setverdict(pass);
			}
		}`
	}
	ops := []struct{ port, op string }{
		{"p1", `p1.send(1);`},
		{"q1", `q1.call(S:{1}, nowait);`},
		{"q1", `q1.call(S:{1}, 0.1) { [] q1.catch(timeout) {} }`},
		{"q1", `q1.reply(S:{1});`},
		{"q1", `q1.raise(S, 1);`},
	}
	for _, o := range ops {
		for _, control := range []string{"stop", "halt"} {
			for _, k := range clocks {
				code := o.port + "." + control + ";"
				v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src(code, o.op))}, "M.tc", k.opts)
				if err != nil {
					t.Fatalf("%s: %v", k.name, err)
				}
				if v != runtime.ErrorVerdict || !strings.Contains(reason, "which is "+control) {
					t.Errorf("%s %s, %s clock: verdict = %s (%s), want error", code, o.op, k.name, v, reason)
				}
			}
			code := o.port + "." + control + "; " + o.port + ".start;"
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src(code, o.op))}, "M.tc", clocks[0].opts)
			if err != nil {
				t.Fatal(err)
			}
			if v != runtime.PassVerdict {
				t.Errorf("%s %s: verdict = %s (%s), want pass", code, o.op, v, reason)
			}
		}
	}
}

// A component's port controlling operations act on its own ports: the MTC
// stopping its port leaves a PTC's port of the same name started.
func TestPortControlIsPerComponent(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		function f_echo() runs on C {
			p.receive(integer:?);
			p.send(2);
		}
		testcase tc() runs on C {
			var C c := C.create;
			connect(self:p, c:p);
			c.start(f_echo());
			p.send(1);
			timer t := 2.0; t.start;
			alt {
				[] p.check(receive(2)) { t.stop }
				[] t.timeout { setverdict(fail, "no echo"); stop }
			}
			p.halt;
			if (not p.checkstate("Halted")) { setverdict(fail, "MTC port not halted"); stop }
			p.receive(2);
			c.done;
			setverdict(pass);
		}
	}`, runtime.PassVerdict)
}

// A port passed as a parameter is the actual port (ETSI 5.4.1.3): its
// procedure operations go to the actual's queue and are errors when the
// actual is stopped or halted, and an element of a port array can be one.
func TestPortControlThroughParameters(t *testing.T) {
	src := func(body string) string {
		return `module M {
			type port P message { inout integer }
			signature S(in integer x) exception (integer);
			type port Q procedure { inout S }
			type component C { port P p1, p2, pa[2]; port Q q1, q2 }
			function f_nowait(Q pq) { pq.call(S:{1}, nowait); }
			function f_block(Q pq) { pq.call(S:{1}, 0.1) { [] pq.catch(timeout) {} } }
			function f_reply(Q pq) { pq.reply(S:{1}); }
			function f_raise(Q pq) { pq.raise(S, 1); }
			function f_send(P pp, integer i) { pp.send(i); }
			function f_stop(P pp) { pp.stop; }
			testcase tc() runs on C {
				connect(self:p1, self:p2);
				connect(self:q1, self:q2);
				connect(self:pa[0], self:pa[1]);
				` + body + `
				setverdict(pass);
			}
		}`
	}
	for _, body := range []string{
		`q1.stop; f_nowait(q1);`,
		`q1.halt; f_block(q1);`,
		`q1.stop; f_reply(q1);`,
		`q1.halt; f_raise(q1);`,
		`p1.stop; f_send(p1, 1);`,
		`f_stop(pa[0]); f_send(pa[0], 1);`,
	} {
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src(body))}, "M.tc", k.opts)
			if err != nil {
				t.Fatalf("%s: %v", k.name, err)
			}
			if v != runtime.ErrorVerdict {
				t.Errorf("%s, %s clock: verdict = %s (%s), want error", body, k.name, v, reason)
			}
		}
	}
	for _, body := range []string{
		`f_nowait(q1);
		timer t := 1.0; t.start;
		alt { [] q2.getcall(S:?) { t.stop } [] t.timeout { setverdict(fail, "the call went elsewhere"); stop } }`,
		`f_send(pa[0], 1);
		timer t := 1.0; t.start;
		alt { [] pa[1].receive(1) { t.stop } [] t.timeout { setverdict(fail, "the message went elsewhere"); stop } }
		all port.stop;
		if (not pa[1].checkstate("Stopped")) { setverdict(fail, "all port.stop left pa[1]"); stop }`,
	} {
		assertSameVerdictBothClocks(t, src(body), runtime.PassVerdict)
	}
}

// A PTC's trigger takes from the PTC's own queue, as the PTC's port state
// lets it (ETSI 22.2.3, 22.5.3): not from the MTC's port of the same name.
func TestPortControlTriggerInAPTC(t *testing.T) {
	for _, stop := range []bool{false, true} {
		ctl, want := "", `[] p.trigger(1) { setverdict(pass) } [else] { setverdict(fail, "trigger missed the PTC's queue") }`
		if stop {
			ctl, want = "p.stop;", `[] p.trigger(integer:?) { setverdict(fail, "trigger matched at a stopped port") } [else] { setverdict(pass) }`
		}
		assertSameVerdictBothClocks(t, `module M {
			type port P message { inout integer }
			type component C { port P p }
			function f() runs on C {
				timer t := 2.0; t.start;
				alt { [] p.check(receive(1)) { t.stop } [] t.timeout { setverdict(fail, "no message"); stop } }
				p.send(7);
				timer w := 0.2; w.start; w.timeout;
				`+ctl+`
				alt { `+want+` }
			}
			testcase tc() runs on C {
				var C c := C.create;
				connect(self:p, c:p);
				p.send(1);
				c.start(f());
				c.done;
			}
		}`, runtime.PassVerdict)
	}
}
