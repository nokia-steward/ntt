package interpreter_test

import "testing"

// A field nested in a structured field changes alone (ETSI 15.5 b).
func TestModifiedTemplate_NestedFieldMerges(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record Inner { integer a, integer b }
		type record Outer { Inner g, integer x }
		template Outer m_o1 := { g := { a := 1, b := 2 }, x := 3 };
		template Outer m_o2 modifies m_o1 := { g := { a := 42 } };
		testcase tc() runs on C {
			if (valueof(m_o2) == { g := { a := 42, b := 2 }, x := 3 }) { setverdict(pass); }
			else { setverdict(fail, m_o2); }
		}
	}`)
}

// A record field left out of a value list, or given "-", is the parent's.
func TestModifiedTemplate_ValueListLeavesTheRest(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer f1 optional, charstring f2, boolean f3 }
		template R m_r1 := { f1 := 123, f2 := "A string", f3 := true };
		template R m_r3 modifies m_r1 := { omit, "A modified string" };
		template R m_r4 modifies m_r1 := { omit, "A modified string", - };
		testcase tc() runs on C {
			var R v_exp := { f1 := omit, f2 := "A modified string", f3 := true };
			if (valueof(m_r3) == v_exp and valueof(m_r4) == v_exp) { setverdict(pass); }
			else { setverdict(fail, m_r3, m_r4); }
		}
	}`)
}

// A record of in value list notation is truncated at its last element,
// "-" takes the parent's; in index notation only the named elements
// change (15.5 c).
func TestModifiedTemplate_RecordOfNotations(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record of integer RoI;
		template RoI m_base := { 0, 1, 2, 3, 4, 5 };
		template RoI m_list modifies m_base := { -, -, 3, 2 };
		template RoI m_idx modifies m_base := { [2] := 3, [3] := 2 };
		testcase tc() runs on C {
			if (valueof(m_list) == { 0, 1, 3, 2 } and valueof(m_idx) == { 0, 1, 3, 2, 4, 5 }) { setverdict(pass); }
			else { setverdict(fail, m_list, m_idx); }
		}
	}`)
}

// A union field is replaced, not merged (15.5 a).
func TestModifiedTemplate_UnionFieldReplaced(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type union U { integer i, charstring s }
		type record R { U u, integer x }
		template R m_1 := { u := { i := 1 }, x := 2 };
		template R m_2 modifies m_1 := { u := { s := "a" } };
		testcase tc() runs on C {
			if (ischosen(m_2.u.s) and not ischosen(m_2.u.i) and valueof(m_2.x) == 2) { setverdict(pass); }
			else { setverdict(fail, m_2); }
		}
	}`)
}
