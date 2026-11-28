package interpreter_test

import (
	"testing"

	"github.com/nokia/ntt/runtime"
)

// A character string rotates by characters, a record of, set of or array
// by elements (ETSI 7.1.7 d, e); the operand is left as it was.
func TestRotateCharacterStringsAndLists(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record of integer RoI;
		type set of integer SoI;
		testcase tc() runs on C {
			var charstring c := "abcdefg";
			var universal charstring u := "aä" & "bc";
			var RoI r := { 1, 2, 3, 4 };
			var SoI s := { 1, 2, 3 };
			var integer a[3] := { 1, 2, 3 };
			if (c <@ 2 == "cdefgab" and c @> 2 == "fgabcde" and c <@ 9 == "cdefgab" and c == "abcdefg" and
			    u <@ 1 == "äbca" and
			    r <@ 1 == { 2, 3, 4, 1 } and r @> 1 == { 4, 1, 2, 3 } and r == { 1, 2, 3, 4 } and
			    s <@ 1 == { 2, 3, 1 } and
			    a @> 1 == { 3, 1, 2 } and
			    "" <@ 3 == "") { setverdict(pass); }
			else { setverdict(fail, c <@ 2, u <@ 1, r <@ 1, a @> 1); }
		}
	}`)
}

// `v++` and `v--` are `v := v + 1` and `v := v - 1` (ETSI 19.1.2), in a
// loop's header too, on a field or an element as on a variable.
func TestShorthandAssignments(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { integer f }
		testcase tc() runs on C {
			var integer n := 0;
			for (var integer i := 0; i < 5; i++) { n++; }
			var R r := { f := 3 };
			r.f--;
			var integer a[2] := { 7, 7 };
			a[1]++;
			n--;
			if (n == 4 and r.f == 2 and a == { 7, 8 }) { setverdict(pass); }
			else { setverdict(fail, n, r, a); }
		}
	}`)
}

// A select case selects when one of its templates matches the value
// (ETSI 19.3.1): a range, a value list or `?` as well as a value.
func TestSelectCaseMatchesTemplates(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		function f(integer x) return charstring {
			var template integer t := (5..10);
			select (x) {
				case (1, 2) { return "small" }
				case (t) { return "range" }
				case ((20, 30)) { return "list" }
				case (?) { return "any" }
			}
			return "none";
		}
		testcase tc() runs on C {
			if (f(2) == "small" and f(7) == "range" and f(30) == "list" and f(99) == "any") { setverdict(pass); }
			else { setverdict(fail, f(2), f(7), f(30), f(99)); }
		}
	}`)
}

// testcasename() is the unqualified name of the testcase that runs, on a
// PTC too (ETSI C.6.2).
func TestTestcasename(t *testing.T) {
	v, reason := runSched(t, "M.tc", `module M {
		type component C {}
		function f() runs on C { if (testcasename() != "tc") { setverdict(fail, testcasename()) } }
		testcase tc() runs on C {
			var C p := C.create;
			p.start(f());
			p.done;
			if (testcasename() == "tc") { setverdict(pass) } else { setverdict(fail, testcasename()) }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}

// __SCOPE__ is the name of the lowest named scope unit around it in the
// source (ETSI D.5): the module, a template, a component type, a testcase
// through its statement blocks, a function, "control".
func TestScopeMacro(t *testing.T) {
	v, reason := runControl(t, "M", `module M {
		const charstring c_m := __SCOPE__;
		template charstring m_t := __SCOPE__;
		type record R { charstring f1, charstring f2 }
		template R m_r(charstring p := __SCOPE__) := { f1 := p, f2 := __SCOPE__ }
		type component C { var charstring v_c := __SCOPE__ }
		function f() return charstring { return __SCOPE__ }
		testcase tc() runs on C {
			var charstring s := "";
			if (true) { s := __SCOPE__ }
			if (c_m == "M" and valueof(m_t) == "m_t" and valueof(m_r()) == { "m_r", "m_r" } and
			    v_c == "C" and s == "tc" and f() == "f") { setverdict(pass) }
			else { setverdict(fail, c_m, m_t, m_r(), v_c, s, f()) }
		}
		control {
			if (__SCOPE__ == "control") { execute(tc()) }
		}
	}`)
	if v != runtime.PassVerdict {
		t.Fatalf("verdict = %s (%s), want pass", v, reason)
	}
}
