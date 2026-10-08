package interpreter_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestLiveClock_TimerReadMeasuresRealElapsed covers real-clock `t.read`
// (the `ntt exec --live` path): a script must be able to measure REAL
// elapsed wall time — `t.start; <operation>; t.read` — which is the enabler
// for profiling a live SUT. Real time is consumed here by an external
// function that sleeps; no `timer.timeout` advances any clock, so the only
// way t.read can report the elapsed is by reading the timer's real StartedAt.
// Under the virtual clock this would read ~0 (see the counterpart below).
func TestLiveClock_TimerReadMeasuresRealElapsed(t *testing.T) {
	runtime.BindExternalFunc("xf_sleep", func([]runtime.Object) runtime.Object {
		time.Sleep(250 * time.Millisecond)
		return runtime.Undefined
	})
	defer runtime.UnbindExternalFunc("xf_sleep")

	src := `module m {
		external function xf_sleep();
		type component C {}
		testcase tc() runs on C system C {
			timer t := 10.0;
			t.start;
			xf_sleep();
			if (t.read >= 0.2) { setverdict(pass); }
			else { setverdict(fail, "t.read did not measure real elapsed on the real clock"); }
		}
	}`
	// Real clock: both deterministic knobs off (the --live execution mode).
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (real-clock t.read must report wall elapsed)", v, reason)
	}
}

// TestLiveClock_VirtualClockIgnoresRealSleep is the counterpart: under the
// deterministic scheduler + virtual clock (the conformance/default mode),
// virtual time advances only at quiescence, so real time spent inside an
// external function is NOT virtual time and t.read reads ~0. This documents
// and locks in the functional (virtual) vs profiling (real) clock split:
// the same script reads elapsed differently by design, per execution mode.
func TestLiveClock_VirtualClockIgnoresRealSleep(t *testing.T) {
	runtime.BindExternalFunc("xf_sleep", func([]runtime.Object) runtime.Object {
		time.Sleep(250 * time.Millisecond)
		return runtime.Undefined
	})
	defer runtime.UnbindExternalFunc("xf_sleep")

	src := `module m {
		external function xf_sleep();
		type component C {}
		testcase tc() runs on C system C {
			timer t := 10.0;
			t.start;
			xf_sleep();
			if (t.read < 0.2) { setverdict(pass); }
			else { setverdict(fail, "virtual t.read counted real sleep as virtual time"); }
		}
	}`
	v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "m.tc",
		interpreter.TestcaseOptions{DeterministicClock: true, DeterministicScheduler: true})
	if err != nil {
		t.Fatalf("RunTestcaseWith: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (virtual t.read must ignore real sleep)", v, reason)
	}
}
