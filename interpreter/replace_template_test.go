package interpreter_test

import "testing"

// replace on a pattern is substr(inpar, 0, index) & repl &
// substr(inpar, index+len, ...), a pattern (ETSI C.4.3).
func TestReplaceTemplate_OnAPatternGivesAPattern(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template charstring v_in := pattern "ab?d";
			var template charstring v_res := replace(v_in, 1, 1, "X");
			if (match("aXzd", v_res) and not match("abzd", v_res)) { setverdict(pass); }
			else { setverdict(fail, v_res); }
		}
	}`)
}

// A pattern repl makes the result a pattern; the value parts match
// themselves.
func TestReplaceTemplate_PatternReplacement(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template charstring v_repl := pattern "x?";
			var template charstring v_res := replace("a?cd", 1, 2, v_repl);
			if (match("axyd", v_res) and not match("a?cd", v_res) and not match("abyd", v_res)) { setverdict(pass); }
			else { setverdict(fail, v_res); }
		}
	}`)
}

// AnyValue as repl is pattern "*" in a character string, a binary string
// of one * in a binary string.
func TestReplaceTemplate_AnyValueReplacement(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template charstring v_c := replace("abcd", 1, 2, ?);
			var template bitstring v_b := replace('0101'B, 1, 2, ?);
			if (match("ad", v_c) and match("axyzd", v_c) and not match("abc", v_c) and
			    match('01'B, v_b) and match('0111'B, v_b) and not match('1101'B, v_b)) { setverdict(pass); }
			else { setverdict(fail, v_c, v_b); }
		}
	}`)
}

// Both AnyValue: ? length (index..infinity); AnyValueOrNone in, AnyValue
// repl: * length (index..infinity).
func TestReplaceTemplate_BothAnyValue(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		testcase tc() runs on C {
			var template charstring v_in := ?, v_none := *, v_repl := ?;
			var template charstring v_a := replace(v_in, 2, 1, v_repl);
			var template charstring v_b := replace(v_none, 2, 1, v_repl);
			if (match("ab", v_a) and match("abcdefg", v_a) and not match("a", v_a) and
			    match("ab", v_b) and not match("a", v_b)) { setverdict(pass); }
			else { setverdict(fail, v_a, v_b); }
		}
	}`)
}

// ifpresent of inpar moves to the result; a fixed-length wildcard counts
// its length in items.
func TestReplaceTemplate_IfpresentAndLength(t *testing.T) {
	runPass(t, "M.tc", `module M {
		type component C {}
		type record R { bitstring f optional }
		testcase tc() runs on C {
			var template bitstring v_in := '0101'B ifpresent;
			var template R v_r := { f := replace(v_in, 1, 2, '11'B) };
			var template charstring v_l := ? length(3);
			var template charstring v_res := replace(v_l, 0, 1, "XY");
			if (match({ f := '0111'B }, v_r) and match({ f := omit }, v_r) and not match({ f := '0101'B }, v_r) and
			    match("XYab", v_res) and not match("XYa", v_res)) { setverdict(pass); }
			else { setverdict(fail, v_r, v_res); }
		}
	}`)
}

// A repl with a matching attribute is none of what C.4.3 allows.
func TestReplaceTemplate_ReplWithLengthIsAnError(t *testing.T) {
	runErrorVerdict(t, "M.tc", `module M {
		type component C {}
		template charstring mw_repl := ? length(2);
		testcase tc() runs on C {
			setverdict(pass);
			var template charstring v_res := replace("abcd", 1, 2, mw_repl);
		}
	}`)
}
