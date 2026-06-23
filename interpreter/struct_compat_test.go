package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/interpreter"
	"github.com/nokia/ntt/runtime"
	"github.com/nokia/ntt/ttcn3"
)

// runPass parses src, runs the named testcase, and asserts a pass
// verdict. Used by the structural-type-compatibility tests below, which
// lock in the ETSI 6.3.2 / 6.2.7 coercion behaviour (field-name remap on
// assignment and out-parameter writeback, and the constrained-array
// index offset) so the conformance-only coverage gains a fast,
// self-contained Go regression test.
func runPass(t *testing.T, name, src string) {
	t.Helper()
	tree := parse(t, src)
	v, reason, err := interpreter.RunTestcase([]*ttcn3.Tree{tree}, name)
	if err != nil {
		t.Fatalf("RunTestcase(%s): %v", name, err)
	}
	if v != runtime.PassVerdict {
		t.Fatalf("%s: verdict = %s (%s), want pass", name, v, reason)
	}
}

// A record/set value returned through an `out` parameter is relabelled
// to the caller lvalue's structurally-compatible type (different field
// names, same shape) - ETSI 6.3.2 / Sem_050402_actual_parameters_184.
func TestStructCompat_OutParamWriteback(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type record R1 { integer field1, integer field2 optional }
		type record R2 { integer elem1, integer elem2 optional }
		type component C {}
		function f_test(out R1 p_val) {
			p_val.field1 := 1;
			p_val.field2 := 2;
		}
		testcase tc() runs on C {
			var R2 v_rec;
			f_test(v_rec);
			if (v_rec == { elem1 := 1, elem2 := 2 }) { setverdict(pass); }
			else { setverdict(fail, v_rec); }
		}
	}`)
}

// `v2 := v1` between structurally-compatible records with different
// field names maps members by position - Sem_060302_structured_types_001
// (record arm). ModifiedRecord's declared order {e,f,g} maps onto
// RecordType {a,b,c}.
func TestStructCompat_AssignmentRecordRemap(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type record RecordType { integer a optional, integer b optional, boolean c }
		type record ModifiedRecord { integer e optional, integer f optional, boolean g }
		type component C {}
		testcase tc() runs on C {
			var ModifiedRecord v1 := { f := 4, e := 8, g := false };
			var RecordType v2;
			v2 := v1;
			if (v2.a == 8 and v2.b == 4 and v2.c == false) { setverdict(pass); }
			else { setverdict(fail, v2); }
		}
	}`)
}

// The same positional remap applies to `set` types (set arm of
// Sem_060302_structured_types_001).
func TestStructCompat_AssignmentSetRemap(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type set SetType { integer a optional, integer b optional, boolean c }
		type set ModifiedSet { integer e optional, integer f optional, boolean g }
		type component C {}
		testcase tc() runs on C {
			var ModifiedSet v1 := { f := 4, e := 8, g := false };
			var SetType v2;
			v2 := v1;
			if (v2.a == 8 and v2.b == 4 and v2.c == false) { setverdict(pass); }
			else { setverdict(fail, v2); }
		}
	}`)
}

// A value assigned to a variable of a constrained array subtype adopts
// the type's declared lower index bound, so `v[1]` reads the first
// element - ETSI 6.2.7 / Sem_060301_non_structured_types_002.
func TestStructCompat_ConstrainedArrayIndexOffset(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type integer ConstrainedInt[1..2];
		type component C {}
		testcase tc() runs on C {
			var integer v_int[2] := { 5, 4 };
			var ConstrainedInt v;
			v := v_int;
			if (v[1] == 5 and v[2] == 4) { setverdict(pass); }
			else { setverdict(fail, v); }
		}
	}`)
}

// The source array keeps its own 0-based offset after the assignment
// (the remap is copy-on-write, not in-place) - regression guard for the
// index-offset adoption above.
func TestStructCompat_ConstrainedArraySourceUnchanged(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type integer ConstrainedInt[1..2];
		type component C {}
		testcase tc() runs on C {
			var integer v_int[2] := { 5, 4 };
			var ConstrainedInt v;
			v := v_int;
			if (v_int[0] == 5 and v_int[1] == 4) { setverdict(pass); }
			else { setverdict(fail, v_int); }
		}
	}`)
}

// Same-type assignment is untouched by the remap (field names already
// match, so the value passes through unchanged). Guards against the
// coercion firing on the common path.
func TestStructCompat_SameTypeAssignmentUnchanged(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type record R { integer a, integer b }
		type component C {}
		testcase tc() runs on C {
			var R v1 := { a := 1, b := 2 };
			var R v2;
			v2 := v1;
			if (v2.a == 1 and v2.b == 2) { setverdict(pass); }
			else { setverdict(fail, v2); }
		}
	}`)
}
