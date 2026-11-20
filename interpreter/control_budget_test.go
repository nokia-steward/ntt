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

// TestControlPartEndsWithItsContext: a control part that loops for good
// stops when the run's context ends, as a testcase does.
func TestControlPartEndsWithItsContext(t *testing.T) {
	src := `module M {
		type component C {}
		testcase quick() runs on C { setverdict(pass) }
		control { var integer i := 0; execute(quick()); while (true) { i := i + 1 } }
	}`
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	var v runtime.Verdict
	var reason string
	go func() {
		defer close(done)
		v, reason, _ = interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", interpreter.TestcaseOptions{Context: ctx})
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the control part did not stop with its context")
	}
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "control part stopped") {
		t.Fatalf("verdict = %s (%s), want error, the control part stopped", v, reason)
	}
}

// TestControlPartsOwnWaits: a control part's alt waits for its timers and
// takes the one due first, on either clock; and on the real clock its time
// running out ends a timer's wait, as it ends a loop.
func TestControlPartsOwnWaits(t *testing.T) {
	src := `module M {
		type component C {}
		testcase tp() runs on C { setverdict(pass) }
		testcase tf() runs on C { setverdict(fail) }
		control {
			var integer which := 0;
			timer t := 2.0, u := 0.2;
			t.start; u.start;
			alt {
				[] t.timeout { which := 1 }
				[] u.timeout { which := 2 }
			}
			if (which == 2 and t.running) { execute(tp()) } else { execute(tf()) }
		}
	}`
	for _, k := range clocks {
		v, reason, _ := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", k.opts)
		if v != runtime.PassVerdict {
			t.Errorf("%s clock: %s (%s), want pass", k.name, v, reason)
		}
	}
	src = `module M {
		type component C {}
		testcase quick() runs on C { setverdict(pass) }
		control { timer w := 30.0; w.start; w.timeout; execute(quick()) }
	}`
	start := time.Now()
	v, reason, _ := interpreter.RunControlWith([]*ttcn3.Tree{parse(t, src)}, "M", interpreter.TestcaseOptions{TestcaseTimeout: 300 * time.Millisecond})
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "did not terminate") {
		t.Errorf("verdict = %s (%s), want error, the control part did not terminate", v, reason)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("took %v, the control part's time is 300ms", time.Since(start))
	}
}
