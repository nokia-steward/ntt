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
