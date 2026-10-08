package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// An `external function` (ETSI 16.1.3) has no body in TTCN-3: the SUT
// adapter supplies one. A binding registered for it runs at the call site
// and its result flows back into the fixture.
func TestExternalFunc_BoundBodyRuns(t *testing.T) {
	runtime.BindExternalFunc("M.xf_answer", func([]runtime.Object) runtime.Object {
		return runtime.NewInt(42)
	})
	defer runtime.UnbindExternalFunc("M.xf_answer")

	runPass(t, "M.tc", `module M {
		type component C {}
		external function xf_answer() return integer;
		testcase tc() runs on C {
			if (xf_answer() == 42) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// The actual parameters reach the bound body in formal-parameter order,
// including through named-argument call syntax.
func TestExternalFunc_ArgumentsReachTheBody(t *testing.T) {
	runtime.BindExternalFunc("M.xf_sum", func(args []runtime.Object) runtime.Object {
		sum := 0
		for _, a := range args {
			i, ok := a.(runtime.Int)
			if !ok {
				return runtime.Errorf("argument is %s, want integer", a.Type())
			}
			sum += int(i.Int64())
		}
		return runtime.NewInt(sum)
	})
	defer runtime.UnbindExternalFunc("M.xf_sum")

	runPass(t, "M.tc", `module M {
		type component C {}
		external function xf_sum(in integer p_a, in integer p_b) return integer;
		testcase tc() runs on C {
			if (xf_sum(1, 2) == 3 and xf_sum(p_b := 4, p_a := 5) == 9) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// A module-qualified binding answers only for that module, so an external
// function of the same name elsewhere stays unbound.
func TestExternalFunc_QualifiedBindingIsScopedToItsModule(t *testing.T) {
	runtime.BindExternalFunc("Other.xf_answer", func([]runtime.Object) runtime.Object {
		return runtime.NewInt(42)
	})
	defer runtime.UnbindExternalFunc("Other.xf_answer")

	if fn, ok := runtime.LookupExternalFunc("M", "xf_answer"); ok {
		t.Fatalf("LookupExternalFunc(M, xf_answer) resolved to %v, want unbound", fn)
	}
	if _, ok := runtime.LookupExternalFunc("Other", "xf_answer"); !ok {
		t.Fatal("LookupExternalFunc(Other, xf_answer): want the binding registered for Other")
	}
}

// An UNBOUND external function keeps yielding Undefined rather than
// raising, which is what lets the hundreds of conformance fixtures that
// declare one without consuming its result still run.
func TestExternalFunc_UnboundCallDoesNotRaise(t *testing.T) {
	tree := parse(t, `module M {
		type component C {}
		external function xf_unbound(in integer p_a);
		testcase tc() runs on C {
			xf_unbound(1);
			setverdict(pass);
		}
	}`)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, "M.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass (an unbound external function must not raise)", v, reason)
	}
}
