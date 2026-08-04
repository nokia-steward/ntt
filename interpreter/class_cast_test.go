package interpreter_test

import (
	"strings"
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

const castClasses = `
		type component C {}
		type class Super {}
		type class Sub extends Super {}
		type class Other {}
`

// `objRef => ClassType` re-types a reference the object satisfies, and
// `objRef of ClassType` tests its runtime class against that class and
// its bases (ETSI 5.1.2.5 / 5.1.2.6, Sem_5010206_Casting_001).
func TestObjectCast_DowncastThenTypeTest(t *testing.T) {
	runPass(t, "M.tc", `module M {`+castClasses+`
		testcase tc() runs on C {
			var Super v_a := Sub.create();
			var Sub v_b := v_a => Sub;
			if (v_b of Sub and v_b of Super and v_a of Sub) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// The `of` test is false for a class the object is unrelated to, and for
// a null reference, which is of no class at all.
func TestObjectCast_OfIsFalseForUnrelatedAndNull(t *testing.T) {
	runPass(t, "M.tc", `module M {`+castClasses+`
		testcase tc() runs on C {
			var Super v_a := Sub.create();
			var Super v_null := null;
			if (not (v_a of Other) and not (v_null of Super)) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// A cast the object does not satisfy is a dynamic error, so the testcase
// reports the bad cast rather than failing an unrelated assertion later.
func TestObjectCast_UnrelatedCastIsAnError(t *testing.T) {
	src := `module M {` + castClasses + `
		testcase tc() runs on C {
			var Super v_a := Super.create();
			var Sub v_b := v_a => Sub;
			setverdict(pass);
		}
	}`
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, "M.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "is not a Sub") {
		t.Fatalf("verdict = %s (%s), want error naming Sub", v, reason)
	}
}

// Casting a null object reference is an error too: there is no object to
// re-type.
func TestObjectCast_NullCastIsAnError(t *testing.T) {
	src := `module M {` + castClasses + `
		testcase tc() runs on C {
			var Super v_a := null;
			var Sub v_b := v_a => Sub;
			setverdict(pass);
		}
	}`
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{parse(t, src)}, "M.tc")
	if err != nil {
		t.Fatalf("RunTestcase: %v", err)
	}
	if v != runtime.ErrorVerdict || !strings.Contains(reason, "null object reference") {
		t.Fatalf("verdict = %s (%s), want error about the null reference", v, reason)
	}
}
