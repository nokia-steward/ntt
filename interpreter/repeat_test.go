package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// TestRepeatOutsideAlt_NoOp keeps the legacy behaviour for stray
// `repeat` statements outside an alt (the spec forbids them but
// existing fixtures shouldn't fail to parse / execute).
//
// Coverage for the in-alt path is the ETSI fixture
// Sem_2003_the_repeat_statement_001.ttcn, which sends two messages
// to a loopback port and uses `repeat` to consume both - exercised
// by `ntt conformance` on the 20_statement_and_operations_for_alt
// directory.
func TestRepeatOutsideAlt_NoOp(t *testing.T) {
	tree := parse(t, `module M {
        type component C {}
        testcase tc_stray() runs on C {
            repeat;
            setverdict(pass);
        }
    }`)
	v, _, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc_stray")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Errorf("verdict = %s, want pass (stray repeat should be a no-op)", v)
	}
}
