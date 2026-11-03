package interpreter_test

import "testing"

// An out parameter does not take its actual's content: it is uninitialised
// when the function is entered (ETSI 3.1, out parameterization, NOTE 3),
// and an actual it was not assigned keeps its value (5.4.2).
func TestOutParameter_UnboundOnEntryAndOnlyAssignedWritesBack(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		function f_read(out integer p) return boolean { return isbound(p); }
		function f_set(out integer p) { p := 7; }
		testcase tc() runs on C {
			var integer kept := 1, written := 1;
			var boolean seen := f_read(kept);
			f_set(written);
			if (not seen and kept == 1 and written == 7) { setverdict(pass); }
			else { setverdict(fail, seen, kept, written); }
		}
	}`)
}

// A variadic parameter is a record of its declared type (ETSI 5.4.3): of
// records, whose actuals take their field names, or of arrays.
func TestVariadicParameter_IsRecordOfItsType(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer f1, integer f2 }
		function f_r(R a...) return integer { return a[0].f1 + a[1].f2; }
		function f_arr(integer a[3]...) return integer { return lengthof(a) * 10 + lengthof(a[0]); }
		testcase tc() runs on C {
			if (f_r({ 1, 2 }, { 3, 4 }) == 5 and f_arr({ 1, 2, 3 }, { 4, 5, 6 }) == 23) { setverdict(pass); }
			else { setverdict(fail); }
		}
	}`)
}

// Integer division truncates towards zero (ETSI 7.1.1).
func TestIntegerDivision_TruncatesTowardsZero(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var integer m := -5, n := 2;
			if (m / n == -2 and 5 / -2 == -2 and m / -2 == 2 and m rem n == -1 and m mod n == 1) { setverdict(pass); }
			else { setverdict(fail, m / n); }
		}
	}`)
}
