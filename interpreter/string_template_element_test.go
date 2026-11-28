package interpreter_test

import "testing"

// An element of AnyValue or AnyValueOrNone of a string type: a single
// AnyElement, or pattern "?", with ifpresent for AnyValueOrNone (ETSI
// 15.6.1 c-f).
func TestStringTemplateElement_OfAnyValue(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { octetstring f optional }
		testcase tc() runs on C {
			var template bitstring v_bit := ?, v_b;
			var template charstring v_char := ?, v_c;
			var template octetstring v_oct := *, v_o;
			v_b := v_bit[0];
			v_c := v_char[0];
			v_o := v_oct[0];
			var template R v_r := { f := v_o };
			if (match('1'B, v_b) and not match('11'B, v_b) and
			    match("a", v_c) and not match("ab", v_c) and not match("", v_c) and
			    match('AA'O, v_o) and match({ f := omit }, v_r)) { setverdict(pass); }
			else { setverdict(fail, v_b, v_c, v_o); }
		}
	}`)
}

// An element read under ifpresent and a length attribute keeps ifpresent.
func TestStringTemplateElement_UnderIfpresentAndLength(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template octetstring v_oct := 'ABCD'O length(2) ifpresent, v_e;
			var template charstring v_char := pattern "a?c" length(3) ifpresent, v_e2;
			v_e := v_oct[1];
			v_e2 := v_char[2];
			if (match('CD'O, v_e) and match("c", v_e2)) { setverdict(pass); }
			else { setverdict(fail, v_e, v_e2); }
		}
	}`)
}

// Past the end of a specific value: AnyElement, or ?, fill the gap; a
// character string becomes a pattern (15.6.1 b, e).
func TestStringTemplateElement_PastTheEnd(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template octetstring v_oct := '0123'O;
			var template charstring v_char := "a?c";
			v_oct[4] := 'FF'O;
			v_char[4] := "x";
			if (match('0123AABBFF'O, v_oct) and not match('0123FF'O, v_oct) and
			    match("a?czx", v_char) and not match("abczx", v_char) and not match("a?cx", v_char)) { setverdict(pass); }
			else { setverdict(fail, v_oct, v_char); }
		}
	}`)
}

// AnyValue and AnyValueOrNone are taken for the empty string;
// AnyValueOrNone adds ifpresent (15.6.1 i, j).
func TestStringTemplateElement_AssignedIntoAnyValue(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { charstring f optional }
		testcase tc() runs on C {
			var template octetstring v_oct := ?;
			var template charstring v_char := *;
			v_oct[2] := 'FF'O;
			v_char[0] := "x";
			var template R v_r := { f := v_char };
			if (match('0102FF'O, v_oct) and not match('FF'O, v_oct) and not match('0102FF03'O, v_oct) and
			    match("x", v_char) and not match("y", v_char) and match({ f := omit }, v_r)) { setverdict(pass); }
			else { setverdict(fail, v_oct, v_char); }
		}
	}`)
}

// A length attribute's maximum grows to index + 1; ifpresent stays
// (15.6.1 k, l).
func TestStringTemplateElement_LengthAndIfpresentKept(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { octetstring f optional }
		testcase tc() runs on C {
			var template octetstring v_oct := 'AB'O length(1..2);
			var template charstring v_char := "abc" length(3);
			var template octetstring v_ip := 'AB'O ifpresent;
			v_oct[3] := 'CD'O;
			v_char[3] := "d";
			v_ip[0] := 'CD'O;
			var template R v_r := { f := v_ip };
			if (match('AB0102CD'O, v_oct) and match("abcd", v_char) and
			    match('CD'O, v_ip) and not match('AB'O, v_ip) and match({ f := omit }, v_r)) { setverdict(pass); }
			else { setverdict(fail, v_oct, v_char, v_ip); }
		}
	}`)
}

// An element of a complemented template list is an error (15.6.1).
func TestStringTemplateElement_OfAComplementIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template charstring v_char := complement("ab");
			setverdict(pass);
			v_char[0] := "x";
		}
	}`)
}

// A value is not a template: past its end is still an error (6.1.1.1).
func TestStringValueElement_PastTheEndIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var charstring v := "abc";
			setverdict(pass);
			v[5] := "z";
		}
	}`)
}
