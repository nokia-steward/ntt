package interpreter_test

import (
	"context"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestLiveClock_StartOfAWaitingPTCDoesNotHoldItsStarter: a started
// behaviour that maps a port is waited for until it has — so that starts
// map in the order they ran — but one that first waits for a message can
// map only once its starter has sent it. Each such start used to cost the
// starter 50 ms.
func TestLiveClock_StartOfAWaitingPTCDoesNotHoldItsStarter(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p, q }
		function f() runs on C {
			var integer x;
			p.receive(integer:?) -> value x;
			map(self:q, system:q);
			p.send(x);
		}
		testcase tc() runs on C system C {
			var C c[20];
			for (var integer i := 0; i < 20; i := i + 1) {
				c[i] := C.create; connect(self:p, c[i]:p); c[i].start(f());
			}
			for (var integer i := 0; i < 20; i := i + 1) { p.send(i) to c[i] }
			timer g := 5.0; g.start;
			var integer n := 0;
			alt {
				[] p.receive(integer:?) { n := n + 1; if (n < 20) { repeat } }
				[] g.timeout { setverdict(fail, n) }
			}
			if (n == 20) { setverdict(pass) }
		}
	}`
	start := time.Now()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", interpreter.TestcaseOptions{})
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), err %v, want pass", v, reason, err)
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("took %v: the starts waited for behaviours that wait for their starter", d)
	}
}

// TestBothClocks_GuardsNoEventAnnounces: an alt waits for an event that
// may make one of its guards true, and for what no event announces it
// looks again now and then — a timer an activated default waits on by its
// parameter's name, one in an altstep the alt calls, a guard that reads a
// timer, a component's end.
func TestBothClocks_GuardsNoEventAnnounces(t *testing.T) {
	for name, body := range map[string]string{
		"default's timer parameter": `
			activate(a(T));
			alt { [] p.receive { setverdict(fail) } [] g.timeout { setverdict(fail, "late") } }`,
		"altstep's timer": `
			alt { [] p.receive { setverdict(fail) } [] b() {} [] g.timeout { setverdict(fail, "late") } }`,
		"guard reads a timer": `
			u.start(0.0); v.start(10.0);
			alt { [v.read > 0.1] u.timeout { setverdict(pass) } [] g.timeout { setverdict(fail, "late") } }`,
		"component's end": `
			var C c := C.create; c.start(w());
			alt { [] c.done { setverdict(pass) } [] g.timeout { setverdict(fail, "late") } }`,
	} {
		src := `module M {
			type port P message { inout integer }
			type component C { port P p; timer T, g, u, v }
			altstep a(timer t) runs on C { [] t.timeout { setverdict(pass) } }
			altstep b() runs on C { [] T.timeout { setverdict(pass) } }
			function w() runs on C { T.start(0.1); T.timeout }
			testcase tc() runs on C system C {
				T.start(0.2); g.start(5.0);` + body + `
			}
		}`
		for _, k := range clocks {
			start := time.Now()
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Fatalf("%s, %s clock: verdict = %s (%s), err %v, want pass", name, k.name, v, reason, err)
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Fatalf("%s, %s clock: took %v", name, k.name, d)
			}
		}
	}
}

// TestBothClocks_TimerWaitIsNotEndedByMessages: messages arriving while a
// component waits for a timer leave the wait, and are all there after it.
func TestBothClocks_TimerWaitIsNotEndedByMessages(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type port P message { inout integer }
		type component C { port P p; timer T }
		function f() runs on C { for (var integer i := 0; i < 100; i := i + 1) { p.send(i) } }
		testcase tc() runs on C system C {
			var C c := C.create; connect(self:p, c:p); c.start(f());
			T.start(0.2); T.timeout;
			if (T.running) { setverdict(fail, "woke early") }
			for (var integer i := 0; i < 100; i := i + 1) {
				alt { [] p.receive(i) {} [] p.receive { setverdict(fail, i) } [else] { setverdict(fail, "missing ", i) } }
			}
			setverdict(pass);
		}
	}`, runtime.PassVerdict)
}

// TestLiveClock_TimerDueDuringTheSnapshot: a timer that comes due while
// the alt looks at its alternatives — here, while a template is
// evaluated against a message it does not match — is taken: the alt
// looks again, rather than waiting for an event that never comes. The
// same for a timer of an activated default.
func TestLiveClock_TimerDueDuringTheSnapshot(t *testing.T) {
	for name, src := range map[string]string{
		"in-line": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function slow() return integer {
				var integer s := 0;
				for (var integer i := 0; i < 30000; i := i + 1) { s := s + 1 }
				return 2
			}
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(1);
				timer t := 0.002; t.start;
				alt { [] t.timeout { setverdict(pass) } [] p.receive(slow()) { setverdict(fail) } }
			}
		}`,
		"any timer": `module M {
			type port P message { inout integer }
			type component C { port P p }
			function slow() return integer {
				var integer s := 0;
				for (var integer i := 0; i < 30000; i := i + 1) { s := s + 1 }
				return 2
			}
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(1);
				timer t := 0.002; t.start;
				alt { [] any timer.timeout { setverdict(pass) } [] p.receive(slow()) { setverdict(fail) } }
			}
		}`,
		"default": `module M {
			type port P message { inout integer }
			type component C { port P p; timer d }
			function slow() return integer {
				var integer s := 0;
				for (var integer i := 0; i < 30000; i := i + 1) { s := s + 1 }
				return 2
			}
			altstep a() runs on C { [] d.timeout { setverdict(pass) } }
			altstep b() runs on C { [] p.receive(slow()) { setverdict(fail) } }
			testcase tc() runs on C system C {
				connect(self:p, self:p);
				p.send(1);
				activate(b()); activate(a());
				d.start(0.002);
				p.receive(integer:5);
			}
		}`,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", interpreter.TestcaseOptions{Context: ctx})
		cancel()
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s: verdict = %s (%s), err %v, want pass", name, v, reason, err)
		}
	}
}

// TestBothClocks_DefaultsTimerParameterOfAnotherName: an activated
// altstep's timer parameter is the actual it was activated with, though
// the activation's scope has a timer of the formal's name.
func TestBothClocks_DefaultsTimerParameterOfAnotherName(t *testing.T) {
	assertSameVerdictBothClocks(t, `module M {
		type port P message { inout integer }
		type component C { port P p }
		altstep a(timer t) { [] t.timeout { setverdict(pass) } }
		testcase tc() runs on C system C {
			timer t;
			timer t2 := 0.2;
			connect(self:p, self:p);
			activate(a(t2));
			t2.start;
			timer g := 3.0; g.start;
			alt { [] p.receive(integer:5) {} [] g.timeout { setverdict(fail, "default not taken") } }
		}
	}`, runtime.PassVerdict)
}

// TestLiveClock_StartWaitsForAComponentThatMapsAfterTimedWait: a
// behaviour that waits for its own timer before it maps — draining its
// port meanwhile — is waited for, as one that maps at once: it maps
// without anything from its starter.
func TestLiveClock_StartWaitsForAComponentThatMapsAfterTimedWait(t *testing.T) {
	for _, wait := range []string{
		"alt { [] p.receive { repeat } [] t.timeout {} }",
		"alt { [] p.receive { repeat } [] ta[0].timeout {} }",
		"activate(a(t)); alt { [] p.receive { repeat } }",
	} {
		testStartWaitsFor(t, wait)
	}
}

func testStartWaitsFor(t *testing.T, wait string) {
	t.Helper()
	src := `module M {
		type port P message { inout integer }
		type component C { port P p, q }
		altstep a(timer t) { [] t.timeout {} }
		function f() runs on C {
			timer t := 0.02, ta[1] := { 0.02 }; t.start; ta[0].start;
			` + wait + `
			map(self:q, system:q);
		}
		testcase tc() runs on C system C {
			timer m := 5.0; m.start;
			var C c := C.create;
			c.start(f());
			var float d := m.read;
			c.done;
			if (d >= 0.015) { setverdict(pass) } else { setverdict(fail, "start ended after ", d, " s, before the map") }
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", interpreter.TestcaseOptions{Context: ctx})
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), err %v, want pass", v, reason, err)
	}
}

// TestLiveClock_DefaultsTimerOfTheComponent: a `runs on` altstep's timer
// is the component's, though a local timer of the function that
// activated it has its name; the alt waits for the component's.
func TestLiveClock_DefaultsTimerOfTheComponent(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p; timer tc }
		altstep a() runs on C { [] tc.timeout { setverdict(pass) } }
		function act() runs on C { timer tc; activate(a()) }
		testcase tc() runs on C system C {
			connect(self:p, self:p);
			act();
			tc.start(0.1);
			p.receive(integer:5);
		}
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", interpreter.TestcaseOptions{Context: ctx})
	if err != nil || v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), err %v, want pass", v, reason, err)
	}
}
