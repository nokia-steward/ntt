package interpreter_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func runControl(t *testing.T, module, src string) (runtime.Verdict, string) {
	t.Helper()
	v, reason, err := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, module,
		interpreter.TestcaseOptions{DeterministicClock: true, DeterministicScheduler: true})
	if err != nil {
		t.Fatalf("RunControlWith(%s): %v", module, err)
	}
	return v, reason
}

// TestControl_SelectsTestcasesByCondition covers the point of running the
// control part at all: it decides WHICH testcases run. Only the passing
// one is reachable, so a runner that just executes the module's first
// testcase reports fail where the module produces pass. Mirrors
// Sem_2602_TheControlPart_001.
func TestControl_SelectsTestcasesByCondition(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_bad() runs on C { setverdict(fail); }
		testcase tc_good() runs on C { setverdict(pass); }
		control {
			if (false) { execute(tc_bad()); }
			if (true) { execute(tc_good()); }
			if (not(1 == 1)) { execute(tc_bad()); }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (only the guarded-true testcase may run)", v, reason)
	}
}

// TestControl_AggregatesWorstVerdict covers ETSI 26.2 aggregation: the
// module's verdict is the worst over the testcases the control part
// actually ran, not the last one's.
func TestControl_AggregatesWorstVerdict(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_fail() runs on C { setverdict(fail); }
		testcase tc_pass() runs on C { setverdict(pass); }
		control {
			execute(tc_fail());
			execute(tc_pass());
		}
	}`)
	if v != runtime.FailVerdict {
		t.Fatalf("verdict = %s (%s), want fail (aggregate keeps the worst verdict)", v, reason)
	}
}

// TestControl_ExecuteReturnsVerdictForBranching covers `v := execute(...)`
// threading a verdict into the next testcase's actual parameter — the
// case static argument mining cannot serve, because the value only
// exists once the control part has run. Mirrors
// Sem_2601_ExecuteStatement_005.
func TestControl_ExecuteReturnsVerdictForBranching(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_first() runs on C { setverdict(pass); }
		testcase tc_second(verdicttype p_verdict) runs on C {
			if (p_verdict == pass) { setverdict(fail); }
			else { setverdict(pass); }
		}
		control {
			var verdicttype v_result;
			v_result := execute(tc_first());
			execute(tc_second(v_result));
		}
	}`)
	if v != runtime.FailVerdict {
		t.Fatalf("verdict = %s (%s), want fail (tc_second must observe tc_first's pass)", v, reason)
	}
}

// TestControl_ExplicitNoneVerdictSurvives guards the distinction between
// "no verdict was declared" and "none was declared". A testcase that
// never calls setverdict resolves to pass (ETSI 22.4.1), but one that
// sets none explicitly must report none, because the control part can
// branch on it. Mirrors Sem_2601_ExecuteStatement_004.
func TestControl_ExplicitNoneVerdictSurvives(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_none() runs on C { setverdict(none); }
		testcase tc_check(verdicttype p_verdict) runs on C {
			if (p_verdict == none) { setverdict(pass); }
			else { setverdict(fail); }
		}
		control {
			var verdicttype v_result;
			v_result := execute(tc_none());
			execute(tc_check(v_result));
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (an explicit setverdict(none) must not resolve to pass)", v, reason)
	}
}

// TestControl_UndeclaredVerdictStaysNone is the other half of the same
// rule: a body that never calls setverdict ends at `none`, the value the
// verdict starts at (ETSI 22.4.1).
//
// This asserted `pass` until 2026-08-10, when the engine coerced an
// undeclared verdict. That reading made a testcase which does nothing look
// successful, and fabricated a verdict for bodies whose setverdict is
// never reached.
func TestControl_UndeclaredVerdictStaysNone(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_silent() runs on C { }
		control { execute(tc_silent()); }
	}`)
	if v != runtime.NoneVerdict {
		t.Fatalf("verdict = %s (%s), want none (a testcase that never sets a verdict has not passed)", v, reason)
	}
}

// TestControl_ExecuteTimeoutYieldsError covers `execute(TC(), d)`: a
// testcase that does not terminate within d is stopped and the execute
// yields error (ETSI 26.1). The body is an empty `while(true){}`, which
// has no statements at which the per-statement stop check could fire, so
// this also guards the loop-level cancellation check. Mirrors
// Sem_2601_ExecuteStatement_007.
func TestControl_ExecuteTimeoutYieldsError(t *testing.T) {
	start := time.Now()
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_spin() runs on C {
			while (true) {
			}
			setverdict(pass);
		}
		control { execute(tc_spin(), 0.3); }
	}`)
	if v != runtime.ErrorVerdict {
		t.Fatalf("verdict = %s (%s), want error (a testcase past its execute timeout is stopped)", v, reason)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("took %s: the execute timeout did not stop the spinning testcase", d)
	}
}

// TestControl_ExecuteUnknownHostYieldsError covers the host operand: a
// host the runtime cannot resolve makes execute yield error without the
// testcase body running. Mirrors Sem_2601_ExecuteStatement_009.
func TestControl_ExecuteUnknownHostYieldsError(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc(integer p_value) runs on C { setverdict(pass); }
		control {
			var integer v_test := 20;
			execute(tc(v_test), -, "no_such_host_2f8a1");
		}
	}`)
	if v != runtime.ErrorVerdict {
		t.Fatalf("verdict = %s (%s), want error (an unresolvable host is an execute error)", v, reason)
	}
}

// TestControl_ExecuteFromTestcaseIsAnError covers ETSI 16.3 / 26.2: a
// testcase can only be executed from the control part or from behaviour
// called from it, so an execute() inside a testcase body is a dynamic
// error rather than a silent no-op.
func TestControl_ExecuteFromTestcaseIsAnError(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc_inner() runs on C { setverdict(fail); }
		testcase tc_outer() runs on C {
			execute(tc_inner());
			setverdict(pass);
		}
		control { execute(tc_outer()); }
	}`)
	if v != runtime.ErrorVerdict {
		t.Fatalf("verdict = %s (%s), want error (execute is not allowed inside a testcase)", v, reason)
	}
	if !strings.Contains(reason, "control part") {
		t.Errorf("reason = %q, want it to explain that execute belongs to the control part", reason)
	}
}

// TestControl_ExecuteViaHelperFunction covers execute() reached through a
// helper called from control. The handler lives on the module scope
// precisely so a function body — which evaluates in its own closure, not
// the control scope — can still find it.
func TestControl_ExecuteViaHelperFunction(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		type component C {}
		testcase tc() runs on C { setverdict(fail); }
		function f_caller() { execute(tc()); }
		control { f_caller(); }
	}`)
	if v != runtime.FailVerdict {
		t.Fatalf("verdict = %s (%s), want fail (execute must work from a helper called by control)", v, reason)
	}
}

// TestControlPartIsLoadBearing pins which shapes route through the
// control part. A lone plain execute is equivalent to running the
// testcase directly, so it stays on the better-exercised direct path.
func TestControlPartIsLoadBearing(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
		src  string
	}{
		{"single plain execute", false, `control { execute(tc()); }`},
		{"two executes", true, `control { execute(tc()); execute(tc()); }`},
		{"execute with timeout", true, `control { execute(tc(), 2.0); }`},
		{"execute with host", true, `control { execute(tc(), -, "h"); }`},
		{"no execute at all", false, `control { var integer v := 1; }`},
	} {
		src := "module M {\n type component C {}\n testcase tc() runs on C { setverdict(pass); }\n" + tc.src + "\n}"
		got := interpreter.ControlPartIsLoadBearing([]*ttcn3.Tree{parse(t, src)}, "M")
		if got != tc.want {
			t.Errorf("%s: load-bearing = %v, want %v", tc.name, got, tc.want)
		}
	}
}
