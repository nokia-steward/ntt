package interpreter_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestTimerRunningReadsTheClock: `while (t.running)` around a function
// that waits on a timer of its own runs until t expires, however often it
// asks — `.running` reads the clock, it does not count the questions.
func TestTimerRunningReadsTheClock(t *testing.T) {
	src := `module M {
		type component C {}
		function waitAWhile(float budget) {
			timer t_line := budget;
			t_line.start;
			alt { [] t_line.timeout {} }
		}
		testcase tc() runs on C {
			timer t_window := 1.0;
			var integer n := 0;
			t_window.start;
			while (t_window.running) {
				var float left := 1.0 - t_window.read;
				if (left > 0.1) { left := 0.1 }
				waitAWhile(left);
				n := n + 1;
			}
			if (n >= 9 and n <= 11) { setverdict(pass) } else { setverdict(fail, n, " iterations") }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestTimerRunningBusyWaitEnds: a busy-wait on `.running` ends — on the
// real clock at the deadline, on the virtual clock, where computing takes
// no time, after a few polls at one instant.
func TestTimerRunningBusyWaitEnds(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C {
			timer t := 0.3;
			t.start;
			while (t.running) {}
			setverdict(pass);
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestAnyTimerRunningReadsTheClock: `any timer.running` too.
func TestAnyTimerRunningReadsTheClock(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C {
			timer t_window := 1.0, t_step;
			var integer n := 0;
			t_window.start;
			while (any timer.running) {
				t_step.start(0.1); t_step.timeout;
				n := n + 1;
			}
			if (n >= 9 and n <= 11) { setverdict(pass) } else { setverdict(fail, n, " iterations") }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestTimerRunningKeepsTheTimeout: asking an expired timer whether it is
// running leaves its timeout pending (ETSI 23.6).
func TestTimerRunningKeepsTheTimeout(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C {
			timer t1 := 0.5, t2 := 1.0, g := 3.0;
			t1.start; t2.start; g.start;
			t2.timeout;
			if (t1.running) { setverdict(fail, "an expired timer runs"); stop }
			alt { [] t1.timeout { setverdict(pass) } [] g.timeout { setverdict(fail, "t1's timeout was lost") } }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestTimerRunningAroundReceives: a loop receiving a burst of messages at
// one instant is not busy-waiting.
func TestTimerRunningAroundReceives(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p }
		function burst() runs on C { for (var integer i := 0; i < 10; i := i + 1) { p.send(i) } }
		testcase tc() runs on C {
			var C c := C.create;
			connect(self:p, c:p);
			c.start(burst());
			timer t := 1.0;
			var integer n := 0;
			t.start;
			while (t.running) {
				alt { [] p.receive { n := n + 1 } [] t.timeout {} }
			}
			if (n == 10) { setverdict(pass) } else { setverdict(fail, n, " received") }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestControlPartTimersOnTheVirtualClock: on the virtual clock a control
// part's timer runs on its own clock, which each testcase it executes
// moves on by the time that testcase took — not on the real clock.
func TestControlPartTimersOnTheVirtualClock(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C { timer tt := 1.0; tt.start; tt.timeout; setverdict(pass) }
		testcase count(integer n) runs on C { if (n == 3) { setverdict(pass) } else { setverdict(fail, n, " executes") } }
		control {
			timer t := 3.0;
			var integer n := 0;
			t.start;
			while (t.running) { execute(tc()); n := n + 1 }
			timer w := 100.0;
			w.start; w.timeout;
			timer a := 50.0;
			a.start;
			alt { [] a.timeout { n := n + 0 } }
			execute(count(n));
		}
	}`
	done := make(chan struct{})
	var v runtime.Verdict
	var reason string
	go func() {
		defer close(done)
		v, reason, _ = interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", interpreter.TestcaseOptions{DeterministicClock: true, DeterministicScheduler: true})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the control part's timers waited in real time")
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// TestTimerRunningPolledInALoop: a loop asking at one instant whether its
// time is up, item by item, works through its items while the timer runs.
func TestTimerRunningPolledInALoop(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C {
			timer t := 10.0;
			var integer k := 0;
			t.start;
			for (var integer i := 0; i < 10; i := i + 1) { if (t.running) { k := k + 1 } }
			if (k == 10 and t.running) { setverdict(pass) } else { setverdict(fail, "k=", k, " read=", t.read) }
		}
	}`
	for _, k := range clocks {
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}
