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

// TestTimeoutOfIdleTimerWaits: a timeout of a timer that is not running —
// never started, stopped, or its timeout already taken — is the alt with
// that one alternative (ETSI 23.6), which nothing can satisfy: it waits,
// stand-alone or in an alt, named, `any timer` or `any from`, rather than
// going on. A PTC that went on would send; the MTC passes when nothing
// comes before its guard timer.
func TestTimeoutOfIdleTimerWaits(t *testing.T) {
	waits := map[string]string{
		"never started": `timer t; t.timeout;`,
		"stopped":       `timer t := 1.0; t.start; t.stop; t.timeout;`,
		"taken":         `timer t := 0.1; t.start; t.timeout; t.timeout;`,
		"in an alt":     `timer t; alt { [] t.timeout {} }`,
		"false guard":   `timer t := 0.1; t.start; alt { [false] t.timeout {} }`,
		"any timer":     `any timer.timeout;`,
		"any from":      `timer ta[2] := {0.1, 0.1}; any from ta.timeout;`,
		"parameter":     `timer t; f(t);`,
	}
	for name, wait := range waits {
		src := `module M {
			type port P message { inout integer }
			type component C { port P p }
			function f(timer t) { t.timeout }
			function idle() runs on C { ` + wait + ` p.send(1) }
			testcase tc() runs on C {
				var C c := C.create;
				connect(c:p, self:p);
				c.start(idle());
				timer g := 0.5;
				g.start;
				alt {
					[] p.receive { setverdict(fail, "the wait went on") }
					[] g.timeout { setverdict(pass) }
				}
				c.stop;
			}
		}`
		for _, k := range clocks {
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", k.opts)
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestTimeoutOfIdleTimerIsADeadlock: with nothing else in the test
// system, the wait never ends. The virtual clock tells so at once, an
// error; the real one, at the testcase's time limit.
func TestTimeoutOfIdleTimerIsADeadlock(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tc() runs on C { timer t; setverdict(pass); t.timeout; setverdict(fail, "went on") }
	}`
	for _, k := range clocks {
		opts := k.opts
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		opts.Context = ctx
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
		cancel()
		want := "deadlocked"
		if !opts.DeterministicScheduler {
			want = "did not terminate"
		}
		if err != nil || v != runtime.ErrorVerdict || !strings.Contains(reason, want) {
			t.Errorf("%s clock: %s (%s) %v, want error, %s", k.name, v, reason, err, want)
		}
	}
}

// TestTimeoutOfIdleTimerTakesItsDefault: an active default applies to the
// wait, as to any alt; one that an event satisfies ends it.
func TestTimeoutOfIdleTimerTakesItsDefault(t *testing.T) {
	src := `module M {
		type port P message { inout integer }
		type component C { port P p; timer g := 0.5; var integer took := 0 }
		altstep onGuard() runs on C { [] g.timeout { took := 1 } }
		altstep onMessage() runs on C { [] p.receive(integer:?) { took := 2 } }
		function later() runs on C { timer w := 0.2; w.start; w.timeout; p.send(1) }
		testcase tc() runs on C {
			timer t;
			g.start;
			var default d := activate(onGuard());
			t.timeout;
			if (took != 1) { setverdict(fail, "went on before the default") }
			deactivate(d);
			d := activate(onMessage());
			var C c := C.create;
			connect(c:p, self:p);
			c.start(later());
			t.timeout;
			if (took != 2) { setverdict(fail, "went on before the message") }
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

// TestAltWaitsForAnIndexedTimer: an alt waits for the timer an element
// of a timer array names, as for a named one — the wait used to see no
// timer in it, and ended, or took the system for deadlocked.
func TestAltWaitsForAnIndexedTimer(t *testing.T) {
	alts := map[string]string{
		"element":           `timer ta[2]; ta[1].start(0.1); alt { [] ta[1].timeout { setverdict(pass) } }`,
		"variable index":    `timer ta[2]; var integer i := 1; ta[i].start(0.1); alt { [] ta[i].timeout { setverdict(pass) } }`,
		"component's array": `cta[1].start; alt { [] cta[1].timeout { setverdict(pass) } }`,
		"two dimensions":    `timer ta[2][2]; ta[1][0].start(0.1); alt { [] ta[1][0].timeout { setverdict(pass) } }`,
	}
	for name, alt := range alts {
		src := `module M {
			type component C { timer cta[2] := {0.1, 0.2} }
			testcase tc() runs on C { ` + alt + ` }
		}`
		for _, k := range clocks {
			opts := k.opts
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			opts.Context = ctx
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
			cancel()
			if err != nil || v != runtime.PassVerdict {
				t.Errorf("%s, %s clock: %s (%s) %v", name, k.name, v, reason, err)
			}
		}
	}
}

// TestAltIndexedByAFuzzyParameter: an index naming a @fuzzy parameter is
// evaluated by the guard, once a snapshot, and not again to find the
// timer to wait for: the alt waits for any of the array's timers.
func TestAltIndexedByAFuzzyParameter(t *testing.T) {
	src := `module M {
		type component C { var integer n := 0 }
		function idx() runs on C return integer { n := n + 1; return 1 }
		function w(@fuzzy integer i) runs on C { timer ta[2]; ta[1].start(0.1); alt { [] ta[i].timeout {} } }
		testcase tc() runs on C { w(idx()); if (n == 2) { setverdict(pass) } else { setverdict(fail, n, " evaluations") } }
	}`
	for _, k := range clocks {
		opts := k.opts
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		opts.Context = ctx
		v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
		cancel()
		if err != nil || v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s) %v", k.name, v, reason, err)
		}
	}
}

// TestControlPartTimeoutOfIdleTimer: a control part that waits for a
// timer that is not running waits until its time is up, and ends there:
// what follows the wait does not run. On the virtual clock that is at
// once: it deadlocked. A timer it can wait for, an array's element too,
// it waits for.
func TestControlPartTimeoutOfIdleTimer(t *testing.T) {
	controls := map[string]string{
		"stand-alone":   `timer t; execute(tp()); t.timeout; execute(tp())`,
		"in an alt":     `timer t; execute(tp()); alt { [] t.timeout {} } log("went on"); execute(tp())`,
		"in a function": `log("x=", f() + 1); execute(tp())`,
		"call guard":    `timer t; alt { [h()] t.timeout {} } execute(tp())`,
	}
	for name, control := range controls {
		src := `module M {
			type component C {}
			testcase tp() runs on C { setverdict(pass) }
			function f() return integer { timer t; t.timeout; return 1 }
			function h() return boolean { return false }
			control { ` + control + ` }
		}`
		for _, k := range clocks {
			opts := k.opts
			opts.TestcaseTimeout = 300 * time.Millisecond
			start := time.Now()
			v, reason, _ := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", opts)
			want := "deadlocked"
			if !opts.DeterministicClock {
				want = "did not terminate"
			}
			if v != runtime.ErrorVerdict || !strings.Contains(reason, want) {
				t.Errorf("%s, %s clock: %s (%s), want error, %s", name, k.name, v, reason, want)
			}
			if time.Since(start) > 10*time.Second {
				t.Errorf("%s, %s clock: took %v", name, k.name, time.Since(start))
			}
		}
	}
	src := `module M {
		type component C {}
		testcase tp() runs on C { setverdict(pass) }
		control { timer ta[2]; ta[1].start(0.2); alt { [] ta[1].timeout { execute(tp()) } } }
	}`
	for _, k := range clocks {
		opts := k.opts
		opts.TestcaseTimeout = 3 * time.Second
		if v, reason, _ := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", opts); v != runtime.PassVerdict {
			t.Errorf("indexed timer, %s clock: %s (%s), want pass", k.name, v, reason)
		}
	}
}
