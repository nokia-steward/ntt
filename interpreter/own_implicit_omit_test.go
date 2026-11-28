package interpreter_test

import "testing"

// A variable whose own with clause gives it the optional attribute
// "implicit omit" has its optional fields, at any depth, set to omit
// where they get no value: when declared, after each assignment and after
// use as an out actual parameter (ETSI 27.7).
func TestOwnImplicitOmit(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R1 { integer a, boolean b optional }
		type record R2 { R1 m }
		function f_out(out R1 p) { p := { a := 1 }; }
		testcase tc() runs on C {
			var R1 v_d := { a := 1 } with { optional "implicit omit" }
			var R2 v_r2 with { optional "implicit omit" }
			var R1 v_o with { optional "implicit omit" }
			var R1 v_plain := { a := 1 };
			v_r2.m.a := 5;
			f_out(v_o);
			if (isbound(v_d.b) and not ispresent(v_d.b) and
			    isbound(v_r2.m.b) and not ispresent(v_r2.m.b) and
			    v_o == { a := 1, b := omit } and not isbound(v_plain.b) and
			    v_d.optional == "implicit omit") { setverdict(pass); }
			else { setverdict(fail, v_d, v_r2, v_o); }
		}
	}`)
}
