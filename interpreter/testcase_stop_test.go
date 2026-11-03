package interpreter_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/runtime/tl"
	"github.com/nokia/ntt/ttcn3"
)

// testcase.stop stops the entire testcase at once with the verdict error,
// from whichever component runs it, its arguments the reason (ETSI 21.2.1):
// the MTC waiting for the PTC that stops does not go on.
func TestTestcaseStop_StopsTheWholeTestcase(t *testing.T) {
	src := `module M {
		type component C {}
		function f_stop() runs on C { testcase.stop("stopped by ", 42); }
		testcase tc() runs on C {
			var C ptc := C.create;
			ptc.start(f_stop());
			ptc.done;
			setverdict(pass);
		}
	}`
	for _, c := range clocks {
		t.Run(c.name, func(t *testing.T) {
			rec := &tl.Recorder{}
			opts := c.opts
			opts.TestLogger = rec
			v, reason, err := interpreter.RunTestcaseWith([]*ttcn3.Tree{parse(t, src)}, "M.tc", opts)
			if err != nil {
				t.Fatalf("RunTestcaseWith: %v", err)
			}
			if v != runtime.ErrorVerdict || !strings.Contains(reason, "stopped by 42") {
				t.Fatalf("verdict = %s (%s), want error (stopped by 42)", v, reason)
			}
			ops := rec.Ops()
			stop := slices.Index(ops, "tliTcStop")
			if stop < 0 {
				t.Fatalf("no tliTcStop in %v", ops)
			}
			if slices.Contains(ops[stop:], "tliSetVerdict") {
				t.Fatalf("the MTC went on after testcase.stop: %v", ops[stop:])
			}
		})
	}
}
