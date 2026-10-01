package interpreter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestOperationSemantics: operations that behaved differently from the
// standard, each with the verdict the standard gives, on both clocks.
func TestOperationSemantics(t *testing.T) {
	for name, src := range map[string]string{
		// Values have value semantics (ETSI 6): an assignment or an
		// initialisation copies, and a field of a record initialised
		// positionally can be assigned.
		"value semantics": `module M {
			type record R { integer a, integer b }
			type component C {}
			testcase tc() runs on C {
				var R r := {1, 2};
				var R r2 := r;
				r.a := 3;
				if (r.a == 3 and r2.a == 1) { setverdict(pass) } else { setverdict(fail, r, r2) }
			}
		}`,
		// What is sent, received, checked or initialised is a value of
		// its own: the sender's later change, a check's redirect, a
		// server's change to a parameter, a component variable's change
		// do not reach anyone else.
		"sent and received values": `module M {
			type record R { integer a, integer b }
			type port P message { inout R }
			signature S(in R r) return R;
			type port Q procedure { inout S }
			const R c_R := { a := 1, b := 2 };
			type component C { port P p; port Q q; var R cr := c_R }
			function rx() runs on C { var R v; p.receive(R:?) -> value v; if (v.a != 1) { setverdict(fail, "received ", v) } }
			function srv() runs on C { var R x; q.getcall(S:{?}) -> param(x); x.a := 42; q.reply(S:{x} value x); }
			function change() runs on C { cr.a := 99; }
			testcase tc() runs on C system C {
				var C c := C.create, s := C.create, m := C.create;
				connect(mtc:p, c:p); connect(mtc:q, s:q);
				var R r := { a := 1, b := 2 };
				p.send(r);
				r.a := 999;
				c.start(rx());
				c.done;
				r.a := 1;
				s.start(srv());
				q.call(S:{r}, 1.0) { [] q.getreply(S:{?} value ?) {} [] q.catch(timeout) { setverdict(fail, "no reply") } }
				if (r.a != 1) { setverdict(fail, "argument changed by the server ", r) }
				m.start(change());
				m.done;
				if (c_R.a != 1) { setverdict(fail, "constant changed ", c_R) }
				disconnect(mtc:p, c:p);
				connect(mtc:p, mtc:p);
				p.send(r);
				var R v, w;
				p.check(receive(R:?) -> value v);
				v.a := 55;
				p.receive(R:?) -> value w;
				if (w.a != 1) { setverdict(fail, "queued message changed by a check redirect ", w) }
				setverdict(pass);
			}
		}`,
		// `system` is the component what arrives from the SUT comes from.
		"system as a value": `module M {
			type port P message { inout integer }
			type component C { port P p }
			testcase tc() runs on C system C {
				map(mtc:p, system:p);
				var C s := system, snd;
				p.send(1);
				p.receive(integer:?) -> sender snd;
				p.send(2);
				timer g := 1.0; g.start;
				alt { [] p.receive(integer:2) from s {} [] g.timeout { setverdict(fail, "from s") } }
				if (snd == system) { setverdict(pass) } else { setverdict(fail, "sender ", snd) }
			}
		}`,
		"action with values": `module M {
			type record R { integer a }
			template R m_r := { a := 5 };
			type component C {}
			testcase tc() runs on C { var integer x := 3; action("send " & m_r & " x" & x); setverdict(pass) }
		}`,
		// A charstring is a value too, and so is what a start or an
		// activate is given: the argument as it was then.
		"charstrings, started and activated": `module M {
			type record R { charstring s, integer x }
			type port P message { inout integer }
			type component C { port P p }
			function modr(inout R r) { r.s[0] := "Q" }
			function started(in R a) { timer t := 0.5; t.start; t.timeout; if (a.x != 1) { setverdict(fail, "start argument changed ", a) } }
			altstep dflt(R a) runs on C { [] p.receive { if (a.x != 1) { setverdict(fail, "activate argument changed ", a) } } }
			testcase tc() runs on C system C {
				var charstring a := "abc";
				var charstring b := a;
				b[0] := "X";
				var R r := { s := "abc", x := 1 };
				var R keep := r;
				modr(r);
				if (a != "abc" or keep.s != "abc" or r.s != "Qbc") { setverdict(fail, a, keep, r) }
				var C c := C.create;
				c.start(started(r));
				var default d := activate(dflt(r));
				r.x := 5;
				connect(self:p, self:p);
				p.send(1);
				timer g := 0.1; g.start;
				alt { [] g.timeout {} }
				c.done;
				setverdict(pass);
			}
		}`,
		// An element of a charstring is assigned where the charstring
		// is: a component variable, an element of an index-range array,
		// through an index evaluated once.
		"charstring elements": `module M {
			type component C { var charstring cs := "abc"; var integer n := 0 }
			type record of charstring L;
			function g() runs on C { cs[0] := "X" }
			function inc() runs on C return integer { n := n + 1; return n }
			testcase tc() runs on C {
				g();
				var charstring a[2..3] := { "abc", "def" };
				a[3][0] := "Q";
				var L l := { "abc", "def" };
				l[inc()][0] := "Z";
				if (cs == "Xbc" and a[3] == "Qef" and a[2] == "abc" and l[1] == "Zef" and lengthof(l) == 2 and n == 1) { setverdict(pass) }
				else { setverdict(fail, cs, a, l, n) }
			}
		}`,
		// A parameter's default and a @lazy parameter are values of the
		// parameter's own: the constant or actual they came from stays.
		"default and lazy parameters": `module M {
			type component C {}
			const charstring cc := "ab";
			function fd(charstring s := cc) return charstring { s[0] := "X"; return s }
			function fl(@lazy charstring s) return charstring { s[0] := "Y"; return s }
			testcase tc() runs on C {
				var charstring a := "ab";
				var charstring r1 := fd(), r2 := fl(a);
				if (cc == "ab" and a == "ab" and r1 == "Xb" and r2 == "Yb") { setverdict(pass) } else { setverdict(fail, cc, a, r1, r2) }
			}
		}`,
		// An altstep taken as an alternative writes its inout parameters
		// back, however often it repeats.
		"altstep alternative inout": `module M {
			type port P message { inout integer }
			type component C { port P p }
			altstep count(inout integer n) runs on C { [] p.receive(integer:?) { n := n + 1; if (n < 3) { repeat } } }
			testcase tc() runs on C {
				connect(self:p, self:p);
				p.send(1); p.send(2); p.send(3);
				var integer n := 0;
				alt { [] count(n) {} }
				if (n == 3) { setverdict(pass) } else { setverdict(fail, "n=", n) }
			}
		}`,
		"param redirect": `module M {
			signature S(in integer x);
			type port PP procedure { inout S }
			type component C { port PP q }
			function srv() runs on C { var integer v; q.getcall(S:?) -> param (v); if (v * 2 == 10) { setverdict(pass) } else { setverdict(fail) } }
			testcase tc() runs on C system C {
				var C s := C.create; connect(self:q, s:q); s.start(srv());
				q.call(S:{5}, nowait); s.done;
			}
		}`,
		// A port index and a `to` clause are evaluated once.
		"evaluated once": `module M {
			type port P message { inout integer }
			type component C { port P p; port P pa[2]; var integer n := 0 }
			function ix() runs on C return integer { n := n + 1; return 0 }
			function to_() runs on C return C { n := n + 1; return self }
			testcase tc() runs on C {
				connect(self:pa[0], self:pa[0]);
				connect(self:p, self:p);
				pa[ix()].send(1);
				p.send(1) to to_();
				if (n == 2) { setverdict(pass) } else { setverdict(fail, n) }
			}
		}`,
		"call to with a block": `module M {
			signature S() return integer;
			type port PP procedure { inout S }
			type component C { port PP q }
			function replier() runs on C { q.getcall(S:{}); q.reply(S:{} value 7); }
			testcase tc() runs on C system C {
				var C s := C.create; connect(self:q, s:q); s.start(replier());
				q.call(S:{}, 2.0) to s {
					[] q.getreply(S:{} value 7) { setverdict(pass) }
					[] q.catch(timeout) { setverdict(fail, "timeout") }
				}
			}
		}`,
		"action": `module M {
			type component C {}
			testcase tc() runs on C { action("press the button"); setverdict(pass) }
		}`,
		"from system": `module M {
			type port P message { inout integer }
			type component C { port P p }
			testcase tc() runs on C system C {
				map(self:p, system:p);
				p.send(1);
				timer g := 1.0; g.start;
				alt { [] p.check(from system) { setverdict(pass) } [] g.timeout { setverdict(fail) } }
			}
		}`,
		// A procedure guard takes a call, never a message; and on `any
		// port` its template applies.
		"any port getcall": `module M {
			signature S(in integer x);
			type port P message { inout integer }
			type port PP procedure { inout S }
			type component C { port P p; port PP q }
			function caller(integer v) runs on C { q.call(S:{v}, nowait); }
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(1);
				timer g := 0.3; g.start;
				alt { [] any port.getcall { setverdict(fail, "a message taken as a call") } [] g.timeout {} }
				var C c := C.create; connect(self:q, c:q); c.start(caller(5));
				timer h := 2.0; h.start;
				alt { [] any port.getcall(S:{5}) { setverdict(pass) } [] h.timeout { setverdict(fail, "S:{5} not taken") } }
			}
		}`,
		// A timer guard whose boolean guard is false does not keep the
		// alt from waiting for the others.
		"false guard": `module M {
			type component C { timer t := 0.1 }
			testcase tc() runs on C {
				var boolean b := false;
				timer g := 1.0; g.start; t.start;
				alt { [b] t.timeout { setverdict(fail, "guarded") } [] g.timeout { setverdict(pass) } }
			}
		}`,
		"done redirect waits": `module M {
			type component C {}
			function f() runs on C { timer w := 0.2; w.start; w.timeout; setverdict(pass); }
			testcase tc() runs on C system C {
				var C c := C.create; var verdicttype v := none;
				c.start(f());
				c.done -> value v;
				if (v != pass) { setverdict(fail, "v=", v) }
			}
		}`,
		// `c.call(f())` runs f on c and waits for it (ETSI 21.3.10): f
		// can wait for what another component sends, and its result and
		// inout parameters come back.
		"call waits for its behaviour": `module M {
			type port P message { inout integer }
			type component C { port P p; var integer x := 0 }
			function feeder() runs on C { timer w := 0.2; w.start; w.timeout; p.send(4); }
			function f(inout integer io) runs on C return integer {
				p.receive(integer:?) -> value x;
				do { io := io + 1 } while (io < 3000);
				return x + 1;
			}
			testcase tc() runs on C system C {
				var C c := C.create, d := C.create;
				connect(c:p, d:p);
				d.start(feeder());
				var integer n := 0;
				var integer r := c.call(f(n));
				if (r == 5 and n == 3000 and x == 0 and not c.running) { setverdict(pass) } else { setverdict(fail, r, n, x) }
			}
		}`,
		// `c.call(f(), d) catch(timeout) {...}`: the block runs only when
		// f did not end within d, and then c is stopped (ETSI 21.3.10).
		"call timeout": `module M {
			type component C { var integer x := 0 }
			function slow() runs on C { timer w := 3.0; w.start; w.timeout; setverdict(fail, "not stopped"); }
			function quick() runs on C { x := 1; }
			testcase tc() runs on C system C {
				var C a := C.create, b := C.create;
				var integer caught := 0;
				a.call(slow(), 0.5) catch(timeout) { caught := caught + 1; }
				b.call(quick(), 1.0) catch(timeout) { caught := caught + 10; }
				timer g := 3.5; g.start; g.timeout;
				if (caught == 1 and not a.running) { setverdict(pass) } else { setverdict(fail, caught) }
			}
		}`,
		// Each element of a port array is a port of its own (ETSI 21.1):
		// connected to a component, it reaches that component only, and
		// what comes back arrives on it.
		"port array elements": `module M {
			type port P message { inout integer }
			type component C { port P p; port P pa[2] }
			function echo() runs on C { var integer v; p.receive(integer:?) -> value v; p.send(v + 1); }
			testcase tc() runs on C system C {
				var C c0 := C.create alive, c1 := C.create;
				connect(self:pa[0], c0:p); connect(self:pa[1], c1:p);
				c0.start(echo()); c1.start(echo());
				pa[1].send(10);
				pa[0].send(20);
				timer g := 2.0; g.start;
				var integer a, b, i := -1, v;
				alt { [] pa[0].receive(integer:?) -> value a {} [] g.timeout { setverdict(fail, "pa[0]"); stop } }
				alt { [] pa[1].receive(integer:?) from c1 -> value b {} [] g.timeout { setverdict(fail, "pa[1]"); stop } }
				c0.done;
				c0.start(echo());
				pa[0].send(30);
				alt { [] any from pa.receive(integer:?) -> value v @index value i {} [] g.timeout { setverdict(fail, "any"); stop } }
				if (a == 21 and b == 11 and i == 0 and v == 31) { setverdict(pass) } else { setverdict(fail, a, b, i, v) }
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

// TestCallTimeoutWithoutCatchIsAnError: a component call that does not end
// in time, with no catch(timeout) clause, ends in a testcase error (ETSI
// 21.3.10).
func TestCallTimeoutWithoutCatchIsAnError(t *testing.T) {
	src := `module M {
		type component C {}
		function slow() runs on C { timer w := 3.0; w.start; w.timeout; }
		testcase tc() runs on C system C { var C a := C.create; setverdict(pass); a.call(slow(), 0.5); }
	}`
	for _, k := range clocks {
		v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if v != runtime.ErrorVerdict {
			t.Errorf("%s clock: %s (%s), want error", k.name, v, reason)
		}
	}
}

// TestCutOffTestcaseIsAnError: a testcase stopped by its time limit did
// not terminate; its verdict is error (ETSI 26), not the one it reached.
func TestCutOffTestcaseIsAnError(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C { setverdict(pass); timer g := 100.0; g.start; g.timeout; }
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", interpreter.TestcaseOptions{Context: ctx})
	if err != nil || v != runtime.ErrorVerdict {
		t.Fatalf("%s (%s) %v, want error", v, reason, err)
	}
}

// TestTeardownIsNotCutOff: a testcase terminates when its MTC does (ETSI
// 26); stopping a PTC that still computes afterwards is no part of it, so
// a limit reached during that is not the testcase's.
func TestTeardownIsNotCutOff(t *testing.T) {
	src := `module M {
		type component C {}
		function spin() runs on C { var integer x := 0; while (true) { x := x + 1 } }
		testcase tc() runs on C system C { var C c := C.create; c.start(spin()); timer g := 0.3; g.start; g.timeout; setverdict(pass); }
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 380*time.Millisecond)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", interpreter.TestcaseOptions{Context: ctx})
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("%s (%s) %v, want pass", v, reason, err)
	}
}

// TestExecuteTimeout: `execute(tc(), d)` stops a testcase that has not
// terminated after d seconds of the test system's time — virtual time on
// the virtual clock — with error (ETSI 26.1), whether the testcase is run
// by its control part or on its own.
func TestExecuteTimeout(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C { setverdict(pass); timer g := 8.0; g.start; g.timeout; }
		control { execute(tc(), 0.5) }
	}`
	for _, k := range clocks {
		start := time.Now()
		v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if v != runtime.ErrorVerdict {
			t.Errorf("run on its own, %s clock: %s (%s), want error", k.name, v, reason)
		}
		v, reason, _ = interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", k.opts)
		if v != runtime.ErrorVerdict {
			t.Errorf("run by the control part, %s clock: %s (%s), want error", k.name, v, reason)
		}
		if k.opts.DeterministicClock && time.Since(start) > 400*time.Millisecond {
			t.Errorf("virtual clock: took %v, the limit is virtual time", time.Since(start))
		}
	}
}

// TestExecuteTimeoutWhileExchanging: two components exchanging messages
// for good never wait long, but time passes as they compute, so an
// execute() limit ends the testcase (ETSI 26.1) on the virtual clock too.
func TestExecuteTimeoutWhileExchanging(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function prod() runs on C { while (true) { p.send(1) } }
		function cons() runs on C { while (true) { p.receive(integer:?) } }
		testcase tc() runs on C system C {
			var C a := C.create, b := C.create; connect(a:p, b:p);
			a.start(prod()); b.start(cons());
			timer t := 60.0; t.start; t.timeout; setverdict(pass);
		}
		control { execute(tc(), 0.5) }
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, k := range clocks {
		opts := k.opts
		opts.Context = ctx
		v, reason, _ := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
		if v != runtime.ErrorVerdict || !strings.Contains(reason, "execute") {
			t.Errorf("%s clock: %s (%s), want the execute() limit", k.name, v, reason)
		}
	}
}
