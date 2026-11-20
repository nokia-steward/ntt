package interpreter_test

import "testing"

// A value list naming fewer fields than the record leaves the others as
// they are, where the optional attribute is "explicit omit" (ETSI
// 6.2.1.0).
func TestShortValueListAssignment(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer f1, integer f2, integer f3 optional }
		testcase tc() runs on C {
			var R v_r := { 1, 2, 3 };
			v_r := { 10 };
			if (v_r == { 10, 2, 3 }) { setverdict(pass); }
			else { setverdict(fail, v_r); }
		}
	}`)
}
