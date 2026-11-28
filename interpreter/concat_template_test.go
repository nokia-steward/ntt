package interpreter_test

import "testing"

// Concatenated with a binary string, AnyValue is one AnyElementsOrNone;
// with a record of, one AnyElementsOrNone, and a wildcard with a length
// one AnyElementsOrNone with that length (ETSI 15.11).
func TestConcatTemplates_WildcardOperands(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record of integer RoI;
		type set of integer SoI;
		template bitstring mw_b := '01'B & ? & '1'B;
		template RoI mw_r := { 1 } & ? & { 3 };
		template RoI mw_l := { 1 } & ? length(2) & { 4 };
		template RoI mw_s := { 1 } & * length(1 .. 2) & { 4 };
		template SoI mw_set := { 1 } & ? length(1);
		testcase tc() runs on C {
			var RoI r13 := { 1, 3 }, r1223 := { 1, 2, 2, 3 }, r12 := { 1, 2 };
			var RoI r1234 := { 1, 2, 3, 4 }, r124 := { 1, 2, 4 }, r14 := { 1, 4 };
			var SoI s := { 2, 1 };
			if (match('011'B, mw_b) and match('010011'B, mw_b) and not match('010'B, mw_b) and
			    match(r13, mw_r) and match(r1223, mw_r) and not match(r12, mw_r) and
			    match(r1234, mw_l) and not match(r124, mw_l) and
			    match(r124, mw_s) and match(r1234, mw_s) and not match(r14, mw_s) and
			    match(s, mw_set)) { setverdict(pass); }
			else { setverdict(fail, mw_b, mw_r, mw_l, mw_s, mw_set); }
		}
	}`)
}
