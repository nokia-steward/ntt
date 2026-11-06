// Tests: bare `timer T := N; T.start; T.timeout;`
// must block for ~N seconds before unwinding. Without the fix
// T.timeout returns immediately and the testcase completes in a
// few microseconds.
package interpreter_test

import (
	"testing"
	"time"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

func TestTimerBareTimeoutBlocks(t *testing.T) {
	tree := parse(t, `module TimerBlockSmoke {
        type component MainCT {}
        testcase tc_Block() runs on MainCT system MainCT {
            timer T := 0.2; T.start; T.timeout;
            setverdict(pass);
        }
    }`)
	start := time.Now()
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "TimerBlockSmoke.tc_Block")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	elapsed := time.Since(start)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s, want pass", v)
	}
	// We slept 0.2 s; allow a generous range to absorb scheduler
	// jitter. Anything under 100 ms means timeout was a no-op.
	if elapsed < 150*time.Millisecond {
		t.Fatalf("testcase completed in %v; want >= ~200 ms (bare timer must block)", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("testcase took %v; want < 2 s (timer must not over-block)", elapsed)
	}
}

func TestTimerAltOnlyTimeoutBlocks(t *testing.T) {
	tree := parse(t, `module TimerAltSmoke {
        type component MainCT {}
        testcase tc_Alt() runs on MainCT system MainCT {
            timer T := 0.2; T.start;
            alt { [] T.timeout { } }
            setverdict(pass);
        }
    }`)
	start := time.Now()
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "TimerAltSmoke.tc_Alt")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	elapsed := time.Since(start)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s, want pass", v)
	}
	if elapsed < 150*time.Millisecond {
		t.Fatalf("testcase completed in %v; want >= ~200 ms (timer-only alt must block)", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("testcase took %v; want < 2 s (timer must not over-block)", elapsed)
	}
}
